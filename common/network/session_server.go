package network

import (
	"errors"
	"fmt"
	"net"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/peng-qing/go_tools/common/options"
)

const (
	defaultMaxConnectionCount = 5000 // 默认最大连接数限制
)

// 设置SessionManager为自定义实现
func WithSessionManager[P any](manager SessionManager[P]) options.Option[SessionServer[P]] {
	return options.WrapperOptions[SessionServer[P]](func(s *SessionServer[P]) {
		s.sessionMgr = manager
	})
}

// SessionServer 接收并管理连接
type SessionServer[P any] struct {
	listener    Listener                   // 监听器
	depsFactory ConnDependenciesFactory[P] // 依赖工厂函数
	connConf    ConnConfig                 // 连接配置
	sessionMgr  SessionManager[P]          // session 管理器
	state       atomic.Uint32              // 生命周期状态
	wg          sync.WaitGroup             // 等待组
	closeOnce   sync.Once                  // 保证只关闭一次
	closeErr    error                      // 关闭错误
}

// NewSessionServer 创建服务端，成功后接管 listener，失败时由调用方关闭
// 工厂可以返回共享依赖或每连接独立依赖，共享对象应保证跨连接并发安全
func NewSessionServer[P any](listener Listener, depsFactory ConnDependenciesFactory[P], connConf ConnConfig, opts ...options.Option[SessionServer[P]]) (*SessionServer[P], error) {
	if listener == nil {
		return nil, errors.New("network/server: listener is nil")
	}
	if depsFactory == nil {
		return nil, errors.New("network/server: deps factory is nil")
	}

	srv := &SessionServer[P]{
		listener:    listener,
		depsFactory: depsFactory,
		connConf:    connConf,
	}
	srv.state.Store(uint32(SessionState_New))
	for _, opt := range opts {
		opt.Apply(srv)
	}
	if srv.sessionMgr == nil {
		srv.sessionMgr = NewSessionManager[P](defaultMaxConnectionCount)
	}

	return srv, nil
}

// Serve 阻塞接收连接，直到监听关闭及所有连接回调完成
func (s *SessionServer[P]) Serve() error {
	// 状态检查
	if !s.state.CompareAndSwap(uint32(SessionState_New), uint32(SessionState_Running)) {
		if s.isClosed() {
			return net.ErrClosed
		}
		return errors.New("network/server: already started")
	}

	defer func() {
		_ = s.Close()
		s.wg.Wait()
	}()

	return s.acceptLoop()
}

// isClosed 服务是否关闭
func (s *SessionServer[P]) isClosed() bool {
	return SessionState(s.state.Load()) == SessionState_Closed
}

// acceptLoop 接收并登记连接，服务关闭时返回 nil，监听异常时返回错误
func (s *SessionServer[P]) acceptLoop() error {
	for {
		if s.isClosed() {
			return nil
		}
		transport, err := s.listener.Accept()
		if err != nil {
			if transport != nil {
				_ = transport.Close()
			}
			if s.isClosed() {
				return nil
			}
			return err
		}
		// Accept 返回期间可能已经发起关闭，不再执行依赖工厂。
		if s.isClosed() {
			if transport != nil {
				_ = transport.Close()
			}
			return nil
		}
		deps, err := s.createDependencies(transport)
		if err != nil {
			if transport != nil {
				_ = transport.Close()
			}
			fmt.Fprintf(os.Stderr, "network/server: create dependencies: %v\n", err)
			continue
		}
		conn, err := newConn(transport, deps, s.connConf)
		if err != nil {
			return err
		}

		if err := s.sessionMgr.Add(conn); err != nil {
			_ = conn.Close()
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			if errors.Is(err, ErrSessionLimitReached) {
				continue
			}
			return err
		}
		// 只有 acceptLoop 增加计数。
		// Serve 在 acceptLoop 退出之后才执行 Wait。
		s.wg.Add(1)
		go s.runConn(conn)
	}
}

// createDependencies 调用依赖工厂，将 panic 转为当前连接的创建错误
func (s *SessionServer[P]) createDependencies(transport Transport) (deps ConnDependencies[P], err error) {
	defer func() {
		if cause := recover(); cause != nil {
			err = fmt.Errorf("network/server: dependencies factory panic: %v\n%s", cause, debug.Stack())
		}
	}()
	deps, err = s.depsFactory(transport)
	if err == nil && deps.Codec == nil {
		err = errors.New("network/server: codec is nil")
	}
	return deps, err
}

// runConn 运行连接 结束后注销
func (s *SessionServer[P]) runConn(conn Connection[P]) {
	defer s.wg.Done()
	defer s.sessionMgr.Remove(conn.ID())

	conn.Run()
}

// Addr 返回监听地址
func (s *SessionServer[P]) Addr() net.Addr {
	return s.listener.Addr()
}

// Conn 获取连接 调用方不应再次调用连接的 Run
func (s *SessionServer[P]) Conn(id uint64) (Connection[P], bool) {
	return s.sessionMgr.Get(id)
}

// Range 遍历连接快照，回调返回 false 时停止
func (s *SessionServer[P]) Range(fn func(Connection[P]) bool) {
	s.sessionMgr.Range(fn)
}

// Len 返回当前登记的连接数
func (s *SessionServer[P]) Len() int {
	return s.sessionMgr.Len()
}

// Close 先关闭已经登记的连接 再关闭监听器 不等待连接运行及业务回调结束
func (s *SessionServer[P]) Close() error {
	s.closeOnce.Do(func() {
		s.state.Store(uint32(SessionState_Closed))
		s.closeErr = s.sessionMgr.Close()
		s.closeErr = errors.Join(s.closeErr, s.listener.Close())
	})
	return s.closeErr
}
