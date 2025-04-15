package pool

import "sync"

type Pool[T any] struct {
	pool sync.Pool
}

// NewPool 创建一个对象池
func NewPool[T any](fn func() T) *Pool[T] {
	return &Pool[T]{
		pool: sync.Pool{
			New: func() any {
				return fn()
			},
		},
	}
}

// Get 对象池获取一个对象
func (p *Pool[T]) Get() T {
	return p.pool.Get().(T)
}

// Put 返回对象池
func (p *Pool[T]) Put(element T) {
	p.pool.Put(element)
}
