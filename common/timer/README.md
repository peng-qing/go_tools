# timer

基于 `container/heap` 的轻量定时器管理器，支持单次和固定间隔任务。

## 回调接口

```go
type TimeOuter interface {
    TimeOut(nowTm int64)
}
```

业务类型实现 `TimeOut` 后即可注册。

## 示例

```go
package main

import (
    "fmt"
    "time"

    "github.com/peng-qing/go_tools/common/timer"
)

type callback struct{}

func (callback) TimeOut(now int64) {
    fmt.Println("timer fired:", now)
}

func main() {
    manager := timer.NewTimerManager(128)

    now := time.Now().UnixMilli()
    id := manager.AddTimer(callback{}, now+1000, 0)
    if id == 0 {
        panic("timer queue is full")
    }

    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()

    for current := range ticker.C {
        _, executed := manager.Run(current.UnixMilli(), 32)
        if executed > 0 {
            break
        }
    }
}
```

## API 语义

- `NewTimerManager(size)`：预分配队列；`size <= 0` 时使用默认容量 1024。
- `AddTimer(callback, end, interval)`：返回定时器 ID；队列满时返回 `0`。
- `RemoveTimer(id)`：按 ID 删除尚未执行的任务。
- `Run(now, limit)`：触发到期任务，返回检查数和执行数；`limit <= 0` 表示不限制本轮执行数量。
- `interval <= 0` 表示单次任务；正数表示每次执行后按当前 `now + interval` 重新调度。

`end`、`interval` 和 `now` 不限定具体单位，但必须完全一致。示例统一使用毫秒。

## 并发说明

`TimerManager` 不包含互斥锁，建议由单一事件循环调用 `AddTimer`、`RemoveTimer` 和 `Run`；如需跨协程访问，由调用方同步。
