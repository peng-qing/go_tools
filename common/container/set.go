package container

import "iter"

// Set 集合
type Set[T comparable] struct {
	data map[T]None
}

// NewSet 创建集合
func NewSet[T comparable]() *Set[T] {
	return &Set[T]{
		data: make(map[T]None),
	}
}

// Empty 判断集合是否为空
func (s *Set[T]) Empty() bool {
	return len(s.data) <= 0
}

// Size 获取集合大小
func (s *Set[T]) Size() int {
	return len(s.data)
}

// Clear 清空集合
func (s *Set[T]) Clear() {
	s.data = make(map[T]None)
}

// Value 获取集合的值
func (s *Set[T]) Value() []T {
	values := make([]T, 0, len(s.data))
	for k := range s.data {
		values = append(values, k)
	}
	return values
}

// Push 添加值到集合中
func (s *Set[T]) Push(val T) {
	s.data[val] = None{}
}

// Contains 判断集合是否包含某个值
func (s *Set[T]) Contains(val T) bool {
	_, ok := s.data[val]
	return ok
}

// Add 添加值到集合中
func (s *Set[T]) Add(values ...T) {
	for _, value := range values {
		s.data[value] = None{}
	}
}

// Remove 从集合中移除某个值
func (s *Set[T]) Remove(val T) {
	delete(s.data, val)
}

// Iter 集合迭代器
func (s *Set[T]) Iter() iter.Seq[T] {
	return func(yield func(T) bool) {
		for k := range s.data {
			if !yield(k) {
				return
			}
		}
	}
}
