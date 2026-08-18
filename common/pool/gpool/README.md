# gpool

提供固定工作者池 `Pool` 和带并发上限的 `TaskRunner`。

## Pool

`NewPool(capacity, jobQueueSize)` 创建固定数量工作者，每个工作者拥有独立任务队列。`Job` 包含：

- `WorkerID`：负数时随机选择工作者；非负数时按 `WorkerID % capacity` 路由，可让同一 ID 的任务保持在同一工作者。
- `Ctx`：传递给处理函数的上下文。
- `Handler`：返回 `error` 的任务函数。

```go
workers := gpool.NewPool(4, 64)

workers.Submit(gpool.Job{
    WorkerID: 10,
    Ctx:      context.Background(),
    Handler: func(ctx context.Context) error {
        return doWork(ctx)
    },
})

workers.Close()
```

`capacity <= 0` 时自动调整为 1；`jobQueueSize < 0` 时调整为 0。通过 `Submit(Job)` 提交任务，`Close()` 会等待已进入工作者队列的任务完成，并可重复调用。`Submit` 不应与 `Close` 并发执行，关闭后不要继续提交。

## TaskRunner

`TaskRunner` 为每个任务启动协程，并通过有界通道限制并发数。

```go
runner := gpool.NewTaskRunner(8, func(ctx context.Context, recovered any) {
    slog.Error("task panic", "value", recovered)
})
defer runner.Close()

err := runner.SubmitImmediately(gpool.Task{
    Ctx: context.Background(),
    TaskFunc: func(ctx context.Context) {
        // do work
    },
})
if errors.Is(err, gpool.ErrTaskRunnerBusy) {
    // 并发额度已满，稍后重试或降级
}
if errors.Is(err, gpool.ErrTaskRunnerClosed) {
    // 执行器已经关闭
}
```

- `Submit` 在并发额度满时阻塞；执行器关闭后直接返回。
- `SubmitImmediately` 不等待，额度满时返回 `ErrTaskRunnerBusy`，关闭后返回 `ErrTaskRunnerClosed`。
- 并发数小于等于 0 时自动调整为 1。
- 自定义 `PanicHandler` 可集中处理任务 panic；传入 `nil` 时使用默认日志处理。
- `Close` 等待已提交任务结束并可重复调用。调用后不得再次提交任务。

任务上下文的取消策略由调用方负责。
