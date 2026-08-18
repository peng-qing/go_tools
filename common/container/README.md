# container

泛型容器和内存缓存实现。

## 组件

| 文件 | 类型 | 用途 |
| --- | --- | --- |
| `queue.go` | `Queue[T]` | FIFO 队列，支持 Push、Pop、Clear、Value |
| `set.go` | `Set[T]` | 集合，支持增删、包含判断和迭代 |
| `sorted_map.go` | `SortedMap[K,V]` | 按有序键维护元素 |
| `lru_cache.go` | `LruCache[K,V]` | 支持绝对过期时间的 LRU |
| `lru_cache.go` | `ShardLruCache[T]` | 按字符串键分片，降低锁竞争 |
| `lru_cache.go` | `LruCache2Q[K,V]` | FIFO 与 LRU 结合的 2Q 缓存 |
| `define.go` | `None` | 无数据占位类型 |

## 示例

```go
package main

import (
    "fmt"
    "time"

    "github.com/peng-qing/go_tools/common/container"
)

func main() {
    set := container.NewSet[string]()
    set.Add("go", "tools")
    fmt.Println(set.Contains("go"))

    queue := container.NewQueue[int]()
    queue.Push(10)
    fmt.Println(queue.Pop())

    cache := container.NewLruCache[string, string](100)
    expiresAt := time.Now().Add(time.Minute).UnixNano()
    cache.Put("language", "Go", expiresAt)

    value, ok := cache.Get("language")
    fmt.Println(value, ok)
}
```

## 缓存选择

- 需要简单的泛型键和值时使用 `LruCache`。
- 字符串键、高并发且希望降低单锁竞争时使用 `ShardLruCache`。
- 希望一次性访问的数据不立即污染主 LRU 时使用 `LruCache2Q`。

`LruCache.Put` 的 `expireAt` 是纳秒级 Unix 时间；传入 `0` 或负数表示不过期。`ShardLruCache` 和 `LruCache2Q` 接收 `time.Duration` 作为默认有效期。

## 并发说明

LRU 系列内部使用互斥锁。`Queue`、`Set` 和 `SortedMap` 不提供并发保护，多协程共享时由调用方同步。
