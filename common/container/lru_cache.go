package container

import (
	"container/list"
	"hash/fnv"
	"sync"
	"time"
)

// 私有化的缓存节点
type cacheEntry[K comparable, V any] struct {
	key      K
	value    V
	expireAt int64
}

// Cache 缓存
type Cache[K comparable, V any] struct {
	list     *list.List          // 双向链表 用于维护 LRU 顺序
	indexMap map[K]*list.Element // 哈希表 用于维护 LRU 顺序
	capacity int                 // 缓存容量
	lock     sync.Mutex          // 互斥锁
}

// NewCache 构造函数
func NewCache[K comparable, V any](capacity int) *Cache[K, V] {
	return &Cache[K, V]{
		list:     list.New(),
		indexMap: make(map[K]*list.Element),
		capacity: capacity,
		lock:     sync.Mutex{},
	}
}

// Put 放入元素到缓存
func (c *Cache[K, V]) Put(key K, val V, expireAt int64) {
	c.lock.Lock()
	defer c.lock.Unlock()

	// 如果键存在 更新值并移动到链表头
	if element, ok := c.indexMap[key]; ok {
		entry := element.Value.(*cacheEntry[K, V])
		entry.value = val
		entry.expireAt = expireAt
		c.list.MoveToFront(element) // 最近访问
		return
	}

	// 如果缓存已满 移除最久未使用节点(末尾节点)
	if len(c.indexMap) >= c.capacity {
		if lastElement := c.list.Back(); lastElement != nil {
			lastEntry := lastElement.Value.(*cacheEntry[K, V])
			delete(c.indexMap, lastEntry.key) // 删除索引
			c.list.Remove(lastElement)        // 移除元素
		}
	}
	// 再插入新节点到表头
	newEntry := &cacheEntry[K, V]{
		key:      key,
		value:    val,
		expireAt: expireAt,
	}
	c.indexMap[key] = c.list.PushFront(newEntry)
}

// 获取缓存信息
func (c *Cache[K, V]) Get(key K) (val V, ok bool) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if element, ok := c.indexMap[key]; ok {
		entry := element.Value.(*cacheEntry[K, V])
		// 如果没过期直接返回
		if entry.expireAt <= 0 || time.Now().UnixNano() < entry.expireAt {
			c.list.MoveToFront(element)
			return entry.value, true
		}
		// 过期了 删除该节点
		delete(c.indexMap, key)
		c.list.Remove(element)
	}

	return
}

// Remove 移除元素
func (c *Cache[K, V]) Remove(key K) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if element, ok := c.indexMap[key]; ok {
		delete(c.indexMap, key)
		c.list.Remove(element)
	}
}

// LruCache Lru 缓存
type LruCache[T any] struct {
	shards     []*Cache[string, T] // 分片缓存 减少锁竞争
	shardCount int                 // 分配数量
	expiration time.Duration       // 默认过期时间
	hashFunc   func(string) int    // 哈希函数 用于寻找分片
}

// NewLruCache 构造函数 创建 lru 缓存
func NewLruCache[T any](bucketCnt int, bucketCapacity int, expiration time.Duration) *LruCache[T] {
	c := &LruCache[T]{
		shards:     make([]*Cache[string, T], bucketCnt),
		shardCount: bucketCnt,
		expiration: expiration,
		// 默认采用 fnv 哈希算法计算键的分片哈希
		hashFunc: func(s string) int {
			h := fnv.New32a()
			_, _ = h.Write([]byte(s))
			return int(h.Sum32())
		},
	}
	for i := 0; i < bucketCnt; i++ {
		c.shards[i] = NewCache[string, T](bucketCapacity)
	}
	return c
}

// SetHashFunc 设置哈希函数
func (lru *LruCache[T]) SetHashFunc(fn func(string) int) {
	lru.hashFunc = fn
}

// getShard 获取目标分片
func (lru *LruCache[T]) getShard(key string) *Cache[string, T] {
	shardIndex := lru.hashFunc(key)
	return lru.shards[shardIndex%lru.shardCount]
}

// Put 添加到缓存
func (lru *LruCache[T]) Put(key string, val T) {
	shard := lru.getShard(key)
	expireAt := int64(0)
	if lru.expiration > 0 {
		expireAt = time.Now().Add(lru.expiration).UnixNano() // 计算过期时间
	}
	shard.Put(key, val, expireAt)
}

// Get 从缓存获取
func (lru *LruCache[T]) Get(key string) (T, bool) {
	shard := lru.getShard(key)
	return shard.Get(key)
}

// Remove 移除缓存
func (lru *LruCache[T]) Remove(key string) {
	shard := lru.getShard(key)
	shard.Remove(key)
}
