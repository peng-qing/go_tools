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

// ShardLruCache 缓存
type LruCache[K comparable, V any] struct {
	list     *list.List          // 双向链表 用于维护 LRU 顺序
	indexMap map[K]*list.Element // 哈希表 用于维护 LRU 顺序
	capacity int                 // 缓存容量
	lock     sync.Mutex          // 互斥锁
}

// NewLruCache 构造函数
func NewLruCache[K comparable, V any](capacity int) *LruCache[K, V] {
	return &LruCache[K, V]{
		list:     list.New(),
		indexMap: make(map[K]*list.Element),
		capacity: capacity,
		lock:     sync.Mutex{},
	}
}

// Put 放入元素到缓存
func (lru *LruCache[K, V]) Put(key K, val V, expireAt int64) {
	lru.lock.Lock()
	defer lru.lock.Unlock()

	lru.put(key, val, expireAt)
}

// put 放入元素到缓存
func (lru *LruCache[K, V]) put(key K, val V, expireAt int64) {
	// 如果键存在 更新值并移动到链表头
	if element, ok := lru.indexMap[key]; ok {
		entry := element.Value.(*cacheEntry[K, V])
		entry.value = val
		entry.expireAt = expireAt
		lru.list.MoveToFront(element) // 最近访问
		return
	}

	// 如果缓存已满 移除最久未使用节点(末尾节点)
	if len(lru.indexMap) >= lru.capacity {
		if lastElement := lru.list.Back(); lastElement != nil {
			lastEntry := lastElement.Value.(*cacheEntry[K, V])
			delete(lru.indexMap, lastEntry.key) // 删除索引
			lru.list.Remove(lastElement)        // 移除元素
		}
	}
	// 再插入新节点到表头
	newEntry := &cacheEntry[K, V]{
		key:      key,
		value:    val,
		expireAt: expireAt,
	}
	lru.indexMap[key] = lru.list.PushFront(newEntry)
}

// Get 获取缓存信息
func (lru *LruCache[K, V]) Get(key K) (val V, ok bool) {
	lru.lock.Lock()
	defer lru.lock.Unlock()

	return lru.get(key)
}

// get 获取缓存信息 私有接口
func (lru *LruCache[K, V]) get(key K) (val V, ok bool) {
	if element, ok := lru.indexMap[key]; ok {
		entry := element.Value.(*cacheEntry[K, V])
		// 如果没过期直接返回
		if entry.expireAt <= 0 || time.Now().UnixNano() < entry.expireAt {
			lru.list.MoveToFront(element)
			return entry.value, true
		}
		// 过期了 删除该节点
		delete(lru.indexMap, key)
		lru.list.Remove(element)
	}

	return
}

// Remove 移除元素
func (lru *LruCache[K, V]) Remove(key K) {
	lru.lock.Lock()
	defer lru.lock.Unlock()

	lru.remove(key)
}

// Remove 移除元素
func (lru *LruCache[K, V]) remove(key K) {
	if element, ok := lru.indexMap[key]; ok {
		delete(lru.indexMap, key)
		lru.list.Remove(element)
	}
}

// ShardLruCache Lru 缓存 分片模式
type ShardLruCache[T any] struct {
	shards     []*LruCache[string, T] // 分片缓存 减少锁竞争
	shardCount int                    // 分配数量
	expiration time.Duration          // 默认过期时间
	hashFunc   func(string) int       // 哈希函数 用于寻找分片
}

// NewShardLruCache 构造函数 创建 lru 缓存
func NewShardLruCache[T any](bucketCnt int, bucketCapacity int, expiration time.Duration) *ShardLruCache[T] {
	c := &ShardLruCache[T]{
		shards:     make([]*LruCache[string, T], bucketCnt),
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
		c.shards[i] = NewLruCache[string, T](bucketCapacity)
	}
	return c
}

// SetHashFunc 设置哈希函数
func (lru *ShardLruCache[T]) SetHashFunc(fn func(string) int) {
	lru.hashFunc = fn
}

// getShard 获取目标分片
func (lru *ShardLruCache[T]) getShard(key string) *LruCache[string, T] {
	shardIndex := lru.hashFunc(key)
	return lru.shards[shardIndex%lru.shardCount]
}

// Put 添加到缓存
func (lru *ShardLruCache[T]) Put(key string, val T) {
	shard := lru.getShard(key)
	expireAt := int64(0)
	if lru.expiration > 0 {
		expireAt = time.Now().Add(lru.expiration).UnixNano() // 计算过期时间
	}
	shard.Put(key, val, expireAt)
}

// Get 从缓存获取
func (lru *ShardLruCache[T]) Get(key string) (T, bool) {
	shard := lru.getShard(key)
	return shard.Get(key)
}

// Remove 移除缓存
func (lru *ShardLruCache[T]) Remove(key string) {
	shard := lru.getShard(key)
	shard.Remove(key)
}

// Lru -2Q 模式
type LruCache2Q[K comparable, V any] struct {
	*LruCache[K, V]               // lru 队列
	fifoQueue       *list.List    // fifo 队列
	expiration      time.Duration // 缓存时间
}

// NewLruCache2Q 构造函数
func NewLruCache2Q[K comparable, V any](capacity int, expiration time.Duration) *LruCache2Q[K, V] {
	return &LruCache2Q[K, V]{
		fifoQueue:  list.New(),
		expiration: expiration,
		LruCache:   NewLruCache[K, V](capacity),
	}
}

// Put 元素放入缓存
func (lru *LruCache2Q[K, V]) Put(key K, value V) {
	lru.lock.Lock()
	defer lru.lock.Unlock()

	// 是否在缓存
	if _, ok := lru.get(key); ok {
		// 放入lru 队首
		lru.put(key, value, time.Now().Add(lru.expiration).UnixNano())
		return
	}
	// 不在缓存 是否是首次加入缓存
	for cursor := lru.fifoQueue.Front(); cursor != nil; cursor = cursor.Next() {
		if entry, ok := cursor.Value.(*cacheEntry[K, V]); ok {
			if entry.key == key {
				// fifo中被二次访问 添加到 lru
				lru.put(key, value, entry.expireAt+int64(lru.expiration))
				// 删除当前
				lru.fifoQueue.Remove(cursor)
				return
			}
		}
	}
	// 首次加入缓存 是否超容量
	if lru.fifoQueue.Len() >= lru.capacity {
		// 移除队尾元素
		lru.fifoQueue.Remove(lru.fifoQueue.Back())
	}
	// 加入队首
	entry := &cacheEntry[K, V]{
		key:      key,
		value:    value,
		expireAt: time.Now().Add(lru.expiration).UnixNano(),
	}
	lru.fifoQueue.PushFront(entry)
}

// Get 从缓存中获取
func (lru *LruCache2Q[K, V]) Get(key K) (val V, ok bool) {
	lru.lock.Lock()
	defer lru.lock.Unlock()

	// 先访问lru 获取
	if val, ok := lru.get(key); ok {
		return val, true
	}
	// 没有 找fifo队列
	for cursor := lru.fifoQueue.Front(); cursor != nil; cursor = cursor.Next() {
		if entry, ok := cursor.Value.(*cacheEntry[K, V]); ok {
			if entry.key == key {
				// fifo中被二次访问 添加到 lru
				lru.put(key, entry.value, entry.expireAt+int64(lru.expiration))
				// 删除当前
				lru.fifoQueue.Remove(cursor)
				return entry.value, true
			}
		}
	}
	// 也没有
	return
}

// Remove 删除缓存
func (lru *LruCache2Q[K, V]) Remove(key K) {
	lru.lock.Lock()
	defer lru.lock.Unlock()

	// 先删lru的
	lru.remove(key)

	// 再删 fifo 队列的
	for cursor := lru.fifoQueue.Front(); cursor != nil; cursor = cursor.Next() {
		if entry, ok := cursor.Value.(*cacheEntry[K, V]); ok {
			if entry.key == key {
				// 删除当前
				lru.fifoQueue.Remove(cursor)
				return
			}
		}
	}
}
