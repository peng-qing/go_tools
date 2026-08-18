# profile

提供基于 `net/http/pprof` 的性能分析服务，并增加强制 GC、内存状态和堆快照接口。

## 启动

```go
package main

import "github.com/peng-qing/go_tools/common/profile"

func main() {
    manager := profile.NewProfileManager()
    manager.ListenProfile("127.0.0.1:6060")

    select {}
}
```

`ListenProfile` 在后台协程启动 HTTP 服务，不阻塞调用方。

## 接口

标准 `net/http/pprof` 路由注册在 `/debug/pprof/`。额外路由：

| 路由 | 作用 |
| --- | --- |
| `/debug/pprof/memory/gc` | 强制执行一次 GC，并返回内存统计 |
| `/debug/pprof/memory/open` | 生成堆 profile 文件并标记分析开始 |
| `/debug/pprof/memory/stop` | 结束分析、关闭文件并返回内存统计 |

堆快照文件名形如 `memory.profile.2026--08-181`，写入进程当前工作目录。应先调用 `open` 再调用 `stop`，不要重复开启。

## 查看 pprof

```bash
go tool pprof http://127.0.0.1:6060/debug/pprof/heap
```

## 安全注意事项

pprof 会暴露进程内部状态，且强制 GC 会影响运行时性能。生产环境应监听本机或受保护的管理网络，并通过防火墙、反向代理和认证限制访问，不要直接暴露到公网。
