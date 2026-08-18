package container

import (
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestQueueLifecycleAndResize(t *testing.T) {
	q := NewQueue[int]()
	if !q.Empty() || q.Size() != 0 {
		t.Fatalf("new queue = empty %v, size %d", q.Empty(), q.Size())
	}
	if got := q.Pop(); got != 0 {
		t.Fatalf("empty Pop() = %d", got)
	}
	for i := 0; i < 1100; i++ {
		q.Push(i)
	}
	if q.capacity <= 1024 {
		t.Fatalf("capacity did not exercise large growth: %d", q.capacity)
	}
	for i := 0; i < 1090; i++ {
		if got := q.Pop(); got != i {
			t.Fatalf("Pop() at %d = %d", i, got)
		}
	}
	actualValues := q.Value()
	expectedValues := []int{1090, 1091, 1092, 1093, 1094, 1095, 1096, 1097, 1098, 1099}
	t.Logf("actual: %v", actualValues)
	t.Logf("expected: %v", expectedValues)
	if !reflect.DeepEqual(actualValues, expectedValues) {
		t.Fatalf("Value() = %v", actualValues)
	}
	if q.capacity >= 1376 {
		t.Fatalf("queue did not shrink: capacity %d", q.capacity)
	}
	for i := 1100; i < 1110; i++ {
		q.Push(i)
	}
	if q.Size() != 20 {
		t.Fatalf("Size() = %d", q.Size())
	}
	q.Clear()
	if !q.Empty() || q.capacity != initCapacity || len(q.Value()) != 0 {
		t.Fatalf("Clear() left queue in unexpected state: %+v", q)
	}
}

func TestSetOperationsAndIterator(t *testing.T) {
	s := NewSet[int]()
	if !s.Empty() {
		t.Fatal("new set is not empty")
	}
	s.Push(3)
	s.Add(1, 2, 2)
	if s.Size() != 3 || !s.Contains(2) || s.Contains(9) {
		t.Fatalf("unexpected set state: %v", s.Value())
	}
	values := s.Value()
	sort.Ints(values)
	expectedValues := []int{1, 2, 3}
	t.Logf("actual: %v", values)
	t.Logf("expected: %v", expectedValues)
	if !reflect.DeepEqual(values, expectedValues) {
		t.Fatalf("Value() = %v", values)
	}
	visited := 0
	for range s.Iter() {
		visited++
		break
	}
	if visited != 1 {
		t.Fatalf("early iterator stop visited %d items", visited)
	}
	s.Remove(2)
	if s.Contains(2) || s.Size() != 2 {
		t.Fatalf("Remove() failed: %v", s.Value())
	}
	s.Clear()
	if !s.Empty() {
		t.Fatal("Clear() failed")
	}
}

func TestSortedMapOrderingMutationAndClear(t *testing.T) {
	m := NewSortedMap[int, string]()
	if m.Begin() != nil || m.Find(1) != nil {
		t.Fatal("empty map returned an element")
	}
	m.Insert(3, "three")
	m.Insert(1, "one")
	m.Insert(2, "two")
	m.Insert(2, "TWO")
	if m.Size() != 3 || !m.Exist(2) || m.Get(2) != "TWO" {
		t.Fatalf("unexpected map state: %#v", m.MapElements())
	}
	var keys []int
	for cursor := m.Begin(); cursor != nil; cursor = cursor.Next {
		keys = append(keys, cursor.Key)
	}
	expectedKeys := []int{1, 2, 3}
	t.Logf("actual: %v", keys)
	t.Logf("expected: %v", expectedKeys)
	if !reflect.DeepEqual(keys, expectedKeys) {
		t.Fatalf("ordered keys = %v", keys)
	}
	if found := m.Find(2); found == nil || found.Key != 2 {
		t.Fatalf("Find(2) = %#v", found)
	}
	m.Delete(1)
	m.Delete(3)
	m.Delete(99)
	if m.Size() != 1 || m.Begin().Key != 2 {
		t.Fatalf("Delete() left keys at %#v", m.Begin())
	}
	m.Clear()
	if m.Size() != 0 || m.Begin() != nil {
		t.Fatal("Clear() failed")
	}
}

func TestLruCacheEvictionUpdateExpirationAndRemove(t *testing.T) {
	lru := NewLruCache[string, int](2)
	lru.Put("a", 1, 0)
	lru.Put("b", 2, 0)
	if got, ok := lru.Get("a"); !ok || got != 1 {
		t.Fatalf("Get(a) = %d, %v", got, ok)
	}
	lru.Put("c", 3, 0)
	if _, ok := lru.Get("b"); ok {
		t.Fatal("least recently used entry was not evicted")
	}
	lru.Put("a", 10, 0)
	if got, ok := lru.Get("a"); !ok || got != 10 {
		t.Fatalf("updated Get(a) = %d, %v", got, ok)
	}
	lru.Put("expired", 4, time.Now().Add(-time.Millisecond).UnixNano())
	if _, ok := lru.Get("expired"); ok {
		t.Fatal("expired entry was returned")
	}
	lru.Remove("a")
	lru.Remove("missing")
	actualValue, actualOK := lru.Get("a")
	t.Logf("actual: value=%d, found=%v", actualValue, actualOK)
	t.Logf("expected: value=0, found=false")
	if actualOK {
		t.Fatal("removed entry was returned")
	}
	defaults := NewLruCache[string, int](0)
	if defaults.capacity != defaultLruCacheCapacity {
		t.Fatalf("default capacity = %d", defaults.capacity)
	}
}

func TestShardLruCacheRoutingExpirationAndRemoval(t *testing.T) {
	cache := NewShardLruCache[int](2, 2, 15*time.Millisecond)
	cache.SetHashFunc(nil)
	cache.SetHashFunc(func(string) int { return -1 })
	cache.Put("key", 7)
	if got, ok := cache.Get("key"); !ok || got != 7 {
		t.Fatalf("Get(key) = %d, %v", got, ok)
	}
	cache.Remove("key")
	if _, ok := cache.Get("key"); ok {
		t.Fatal("removed shard entry was returned")
	}
	cache.Put("expires", 8)
	time.Sleep(25 * time.Millisecond)
	actualValue, actualOK := cache.Get("expires")
	t.Logf("actual: value=%d, found=%v", actualValue, actualOK)
	t.Logf("expected: value=0, found=false")
	if actualOK {
		t.Fatal("expired shard entry was returned")
	}
	defaults := NewShardLruCache[int](0, 0, 0)
	if defaults.shardCount != defaultShardLruCacheBucketCount {
		t.Fatalf("default shard count = %d", defaults.shardCount)
	}
}

func TestLruCache2QPromotionCapacityExpirationAndRemoval(t *testing.T) {
	cache := NewLruCache2Q[string, int](2, 20*time.Millisecond)
	cache.Put("a", 1)
	cache.Put("b", 2)
	cache.Put("c", 3)
	if _, ok := cache.Get("a"); ok {
		t.Fatal("oldest FIFO entry was not evicted")
	}
	if got, ok := cache.Get("b"); !ok || got != 2 {
		t.Fatalf("first Get(b) = %d, %v", got, ok)
	}
	cache.Put("b", 20)
	if got, ok := cache.Get("b"); !ok || got != 20 {
		t.Fatalf("promoted Get(b) = %d, %v", got, ok)
	}
	cache.Remove("b")
	cache.Remove("c")
	cache.Remove("missing")
	if _, ok := cache.Get("b"); ok {
		t.Fatal("removed 2Q entry was returned")
	}
	cache.Put("expires", 9)
	time.Sleep(30 * time.Millisecond)
	actualValue, actualOK := cache.Get("expires")
	t.Logf("actual: value=%d, found=%v", actualValue, actualOK)
	t.Logf("expected: value=0, found=false")
	if actualOK {
		t.Fatal("expired FIFO entry was returned")
	}
}
