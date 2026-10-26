package network

import (
	"errors"
	"net"
	"sync"
)

var (
	_ SessionManager[any] = (*SessionManagerImpl[any])(nil)
)

// SessionManagerImpl 管理连接登记与关闭
type SessionManagerImpl[P any] struct {
	conns     map[uint64]Connection[P] // 连接表
	maxConns  int                      // 最大连接数限制 <=0 表示不额外限制
	closed    bool                     // 关闭标记
	lock      sync.RWMutex             // 锁 保护连接表和关闭
	closeOnce sync.Once                // 保证只关闭一次
	closeErr  error                    // 关闭错误
}

// NewSessionManager 构造函数
func NewSessionManager[P any](maxConns int) *SessionManagerImpl[P] {
	return &SessionManagerImpl[P]{
		conns:    make(map[uint64]Connection[P]),
		maxConns: maxConns,
	}
}

// Add 登记连接
func (s *SessionManagerImpl[P]) Add(conn Connection[P]) error {
	if conn == nil {
		return errors.New("network/session: connections is nil")
	}
	id := conn.ID()
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.closed {
		return net.ErrClosed
	}
	if _, ok := s.conns[id]; ok {
		return ErrSessionIdExists
	}
	if s.maxConns > 0 && len(s.conns) >= s.maxConns {
		return ErrSessionLimitReached
	}
	s.conns[id] = conn
	return nil
}

// Remove 注销连接登记
func (s *SessionManagerImpl[P]) Remove(id uint64) {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.conns, id)
}

// Get 获取已登记的连接
func (s *SessionManagerImpl[P]) Get(id uint64) (Connection[P], bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	conn, ok := s.conns[id]
	return conn, ok
}

// Range 遍历连接快照，回调期间不持锁
func (s *SessionManagerImpl[P]) Range(fn func(Connection[P]) bool) {
	if fn == nil {
		return
	}

	s.lock.RLock()
	conns := make([]Connection[P], 0, len(s.conns))
	for _, conn := range s.conns {
		conns = append(conns, conn)
	}
	s.lock.RUnlock()

	for _, conn := range conns {
		if !fn(conn) {
			return
		}
	}
}

// Len 返回当前登记的连接数
func (s *SessionManagerImpl[P]) Len() int {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return len(s.conns)
}

// Close 停止登记并关闭已有连接，不等待连接运行及业务回调结束
func (s *SessionManagerImpl[P]) Close() error {
	s.closeOnce.Do(func() {
		s.lock.Lock()
		s.closed = true

		conns := make([]Connection[P], 0, len(s.conns))
		for _, conn := range s.conns {
			conns = append(conns, conn)
		}
		s.lock.Unlock()

		// 不持锁关闭，避免阻塞连接查询和注销。
		for _, conn := range conns {
			if err := conn.Close(); err != nil {
				s.closeErr = errors.Join(s.closeErr, err)
			}
		}
	})

	return s.closeErr
}
