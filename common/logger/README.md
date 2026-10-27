# logger

`logger` 提供并发安全的文件日志写入和轮转能力。日志达到指定大小时，当前文件会被重命名为带时间戳的备份，并继续使用原文件名创建新的活动日志。旧备份可按数量或保存时间清理，也可压缩为 gzip 文件。

## 快速开始

```go
package main

import (
    "log"

    "github.com/peng-qing/go_tools/common/logger"
)

func main() {
    writer := logger.NewLogFileRollover(
        "./logs/server.log",
        100,  // 单个日志文件最大 100 MB
        10,   // 最多保留 10 个备份
        30,   // 备份最多保留 30 天
        true, // 使用 gzip 压缩备份
    )
    defer writer.Close()

    appLogger := log.New(writer, "", log.LstdFlags|log.Lmicroseconds)
    appLogger.Println("server started")

    if err := writer.Sync(); err != nil {
        appLogger.Printf("sync log failed: %v", err)
    }
}
```

父目录不存在时会自动创建。`LogFileRollover` 实现了 `io.Writer` 和 `io.Closer`，并提供 `Sync` 方法，可直接交给标准库 `log.Logger` 或其他接受 `io.Writer` 的日志组件。

## 配置

构造函数：

```go
func NewLogFileRollover(
    filename string,
    maxSize int,
    maxBackups int,
    maxAge int,
    compress bool,
) *LogFileRollover
```

| 参数 | 含义 | 零值行为 |
| --- | --- | --- |
| `filename` | 活动日志文件路径 | 写入系统临时目录，文件名为 `${程序名}-rollover.log` |
| `maxSize` | 单个文件的大小上限，单位 MB | 使用默认上限 100 MB |
| `maxBackups` | 最多保留的备份数量 | 不按数量清理 |
| `maxAge` | 备份保存天数，每天按 24 小时计算 | 不按时间清理 |
| `compress` | 是否将备份压缩为 gzip | 保留未压缩备份 |

当 `maxBackups` 和 `maxAge` 同时启用时，备份需要同时满足数量和时间限制。配置字段应在开始写入前完成设置，运行期间不要并发修改。

## 文件轮转

假设活动日志为：

```text
logs/server.log
```

轮转后的文件名类似：

```text
logs/server_2026-10-27T11-30-45.123.log
```

启用压缩后会生成：

```text
logs/server_2026-10-27T11-30-45.123.log.gz
```

备份名中的时间使用服务器本地时区。每次 `Write` 会在锁内完成大小判断和文件写入，因此同一个 `LogFileRollover` 可以被多个 goroutine 并发使用。

单次写入不能大于 `maxSize`：

```go
if _, err := writer.Write(data); err != nil {
    // data 大于单文件上限时返回 logger: write too large
}
```

如果业务需要在文件未达到大小上限时主动切换日志，可以调用：

```go
if err := writer.Rotate(); err != nil {
    log.Printf("rotate log failed: %v", err)
}
```

## 生命周期

- `Write`：追加日志；达到大小限制时先轮转，再写入新文件。
- `Rotate`：立即关闭当前活动文件并创建新文件。
- `Sync`：将已经写入的数据同步到磁盘；首次写入前调用会直接返回 `nil`。
- `Close`：关闭活动文件并停止后台轮转任务，程序退出前应调用。

备份压缩和清理在后台执行。`Write` 或 `Rotate` 返回时，相关 `.gz` 文件或过期备份可能尚未处理完成。当前实现不会将后台压缩、删除错误返回给调用方。

## 使用限制

- 一个活动日志文件应只由一个 `LogFileRollover` 实例和一个进程管理；该包不提供跨进程文件锁。
- 备份文件名精确到毫秒。正常按文件大小轮转时通常不会产生冲突，应避免业务代码在同一毫秒内连续调用 `Rotate`。
- 调用 `Close` 后不要继续调用 `Write`、`Rotate` 或 `Sync`。
- 不要手动改名为相同前缀和时间格式的文件，否则它可能被识别为当前日志的备份并参与清理。

## 测试

```bash
go test -count=1 -race ./common/logger
```
