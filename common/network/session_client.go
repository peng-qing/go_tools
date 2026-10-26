package network

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"

	"github.com/peng-qing/go_tools/common/container"
)

// SessionClient 管理一次拨号及其连接生命周期 不自动重连
type SessionClient[P any] struct {
	dialer     Dialer              // 拨号器
	deps       ConnDependencies[P] // 相关依赖
	connConf   ConnConfig          // 连接配置
	state      atomic.Uint32       // 生命周期状态
	lock       sync.Mutex          // 锁
	dialCancel context.CancelFunc  // 取消正在进行的拨号
	conn       Connection[P]       // 当前连接
	done       chan container.None // 拨号失败或连接及回调结束
	waitErr    error               // 关闭done前写入
	closeOnce  sync.Once           // 保证关闭
	closeErr   error               // 关闭错误
}

// NewSessionClient 创建客户端 每个实例只允许拨号一次
func NewSessionClient[P any](dialer Dialer, deps ConnDependencies[P], connConf ConnConfig) (*SessionClient[P], error) {
	if dialer == nil {
		return nil, errors.New("network/client: dialer is nil")
	}
	if deps.Codec == nil {
		return nil, errors.New("network/client: codec is nil")
	}

	client := &SessionClient[P]{
		dialer:   dialer,
		deps:     deps,
		connConf: connConf,
		done:     make(chan container.None),
	}
	client.state.Store(uint32(SessionState_New))
	return client, nil
}

// DialContext 进行拨号 ctx只用于限制拨号 不得重复拨号
func (c *SessionClient[P]) DialContext(ctx context.Context, address string) (Connection[P], error) {
	if ctx == nil {
		ctx = context.Background()
	}
	c.lock.Lock()
	if !c.state.CompareAndSwap(uint32(SessionState_New), uint32(SessionState_Running)) {
		err := errors.New("network/client: dial already called")
		if c.isClosed() {
			err = net.ErrClosed
		}
		c.lock.Unlock()
		return nil, err
	}
	dialCtx, ctxCancel := context.WithCancel(ctx)
	c.dialCancel = ctxCancel
	c.lock.Unlock()
	defer func() {
		ctxCancel()
		c.lock.Lock()
		c.dialCancel = nil
		c.lock.Unlock()
	}()

	// 拨号在锁外执行，让 Close 可以取消正在进行的拨号。
	rwc, err := c.dialer.DialContext(dialCtx, address)
	if err != nil {
		if rwc != nil {
			_ = rwc.Close()
		}
		c.finish(err)
		return nil, err
	}
	conn, err := newConn(rwc, c.deps, c.connConf)
	if err != nil {
		if rwc != nil {
			_ = rwc.Close()
		}
		c.finish(err)
		return nil, err
	}

	// 检查关闭状态与连接发布一起完成 避免 Close 漏掉新连接
	c.lock.Lock()
	c.dialCancel = nil

	var publishErr error
	if c.isClosed() {
		publishErr = net.ErrClosed
	} else {
		publishErr = dialCtx.Err()
	}
	if publishErr == nil {
		c.conn = conn
	}
	c.lock.Unlock()

	if publishErr != nil {
		// 回收已经构造但因关闭或取消而未能发布的连接
		_ = conn.Close()
		c.finish(publishErr)
		return nil, publishErr
	}

	go c.runConn(conn)

	return conn, nil
}

// Wait 等待拨号失败或者连接结束 会阻塞
func (c *SessionClient[P]) Wait() error {
	if SessionState_New == SessionState(c.state.Load()) {
		return errors.New("network/client: Dial has not been called")
	}
	<-c.done
	return c.waitErr
}

// Close 取消拨号并关闭当前连接
func (c *SessionClient[P]) Close() error {
	c.closeOnce.Do(func() {
		c.lock.Lock()
		prevState := SessionState(c.state.Swap(uint32(SessionState_Closed)))
		cancel, conn := c.dialCancel, c.conn
		c.dialCancel = nil
		c.lock.Unlock()

		if cancel != nil {
			cancel()
		}
		if conn != nil {
			c.closeErr = conn.Close()
		}
		// 尚未拨号时 没有其他执行路径负责发布结束结果
		if prevState == SessionState_New {
			c.finish(net.ErrClosed)
		}
	})
	return c.closeErr
}

// isClosed 客户端是否已发起关闭
func (c *SessionClient[P]) isClosed() bool {
	return SessionState(c.state.Load()) == SessionState_Closed
}

// runConn 运行连接 在连接及其回调结束后发布结果
func (c *SessionClient[P]) runConn(conn *Conn[P]) {
	conn.Run()
	c.finish(conn.Err())
}

// finish 发布结束结果
func (c *SessionClient[P]) finish(err error) {
	c.waitErr = err
	close(c.done)
}
