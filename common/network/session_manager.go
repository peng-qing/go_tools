package network

import (
	"maps"
	"slices"
	"sync"
)

// SessionManager 管理所有连接
type SessionManager[P any] struct {
	allConns map[uint64]Connection[P] // 所有连接
	mu       sync.RWMutex             // 读写锁
}

// NewSessionManager 创建一个新的 SessionManager
func NewSessionManager[P any]() *SessionManager[P] {
	return &SessionManager[P]{allConns: make(map[uint64]Connection[P])}
}

// add 添加一个连接到 SessionManager
func (s *SessionManager[P]) add(conn Connection[P]) error {
	id := conn.ID()
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.allConns[id]; exists {
		return ErrConnectionExists
	}

	if s.allConns == nil {
		s.allConns = make(map[uint64]Connection[P])
	}

	s.allConns[id] = conn
	return nil
}

// remove 从 SessionManager 中移除一个连接
func (s *SessionManager[P]) remove(id uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.allConns, id)
}

// Get 返回瞬时引用，未找到时返回 nil；返回后连接可能立即关闭。
func (s *SessionManager[P]) Get(id uint64) Connection[P] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.allConns[id]
}

// Count 返回当前连接数
func (s *SessionManager[P]) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.allConns)
}

// Snapshot 返回当前所有连接的快照
func (s *SessionManager[P]) Snapshot() []Connection[P] {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.AppendSeq(make([]Connection[P], 0, len(s.allConns)), maps.Values(s.allConns))
}
