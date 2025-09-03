package container

import "cmp"

// KeyElement key结构
type KeyElement[K cmp.Ordered] struct {
	Key  K
	Next *KeyElement[K]
}

// SortedMap 有序Map
type SortedMap[K cmp.Ordered, V any] struct {
	mapElements map[K]V
	ptrHead     *KeyElement[K]
}

// NewSortedMap 构造函数
func NewSortedMap[K cmp.Ordered, V any]() *SortedMap[K, V] {
	return &SortedMap[K, V]{
		mapElements: make(map[K]V),
		ptrHead:     nil,
	}
}

// MapElements 获取所有的元素
func (s *SortedMap[K, V]) MapElements() map[K]V {
	return s.mapElements
}

// Size 获取元素个数
func (s *SortedMap[K, V]) Size() int {
	return len(s.mapElements)
}

// Clear 清空所有元素
func (s *SortedMap[K, V]) Clear() {
	s.mapElements = make(map[K]V)
	s.ptrHead = nil
}

// Begin 获取第一个元素
func (s *SortedMap[K, V]) Begin() *KeyElement[K] {
	return s.ptrHead
}

// Exist 判断key释放存在
func (s *SortedMap[K, V]) Exist(key K) bool {
	_, exist := s.mapElements[key]
	return exist
}

// Find 找到某个Key的位置
func (s *SortedMap[K, V]) Find(key K) *KeyElement[K] {
	cursor := s.ptrHead
	if cursor == nil {
		return nil
	}
	for i := 0; i < s.Size(); i++ {
		if cursor.Key == key {
			return cursor
		}
		cursor = cursor.Next
	}
	return nil
}

// Get 获取map元素
func (s *SortedMap[K, V]) Get(key K) V {
	return s.mapElements[key]
}

// Delete 删除某个元素
func (s *SortedMap[K, V]) Delete(key K) {
	if _, ok := s.mapElements[key]; ok {
		delete(s.mapElements, key)
		cursor := s.ptrHead
		if cursor == nil {
			return
		}
		if cursor.Key == key {
			s.ptrHead = cursor.Next
			return
		}
		for i := 0; i < s.Size(); i++ {
			if cursor.Next == nil {
				break
			}
			if cursor.Next.Key == key {
				cursor.Next = cursor.Next.Next
				break
			}
			cursor = cursor.Next
		}
	}
}

// Insert 插入元素
func (s *SortedMap[K, V]) Insert(key K, val V) {
	_, ok := s.mapElements[key]
	s.mapElements[key] = val
	if !ok {
		newElem := &KeyElement[K]{
			Key: key,
		}
		cursor := s.ptrHead
		if cursor == nil {
			s.ptrHead = newElem
			return
		}
		// 如果head不小于插入值 则替换当前head
		if cursor.Key >= key {
			newElem.Next = cursor
			s.ptrHead = newElem
			return
		}
		for i := 0; i < s.Size(); i++ {
			if cursor.Next == nil {
				cursor.Next = newElem
				break
			}
			if cursor.Next.Key >= key {
				newElem.Next = cursor.Next
				cursor.Next = newElem
				break
			}
			cursor = cursor.Next
		}
	}
}
