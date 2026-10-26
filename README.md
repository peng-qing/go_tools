# go_tools

`go_tools` 是一个面向 Go 服务端开发的通用工具库，提供容器、HTTP、MongoDB、网络通信、对象池、性能分析和定时器等常用能力。

## 环境要求

- Go 1.24.1 或更高版本
- 使用 MongoDB 模块时，需要可访问的 MongoDB 服务

## 安装

```bash
go get github.com/peng-qing/go_tools
```

各能力以独立 Go 包提供，请只导入业务所需的包。

## 模块导航

| 模块 | 说明 |
| --- | --- |
| [common](common/README.md) | 通用组件总览 |
| [container](common/container/README.md) | 泛型容器与缓存 |
| [encode_utils](common/encode_utils/README.md) | 文本编码与解码 |
| [http_utils](common/http_utils/README.md) | HTTP 请求与响应 |
| [mongo_driver](common/mongo_driver/README.md) | MongoDB 连接与 CRUD |
| [network](common/network/README.md) | TCP/KCP/WebSocket、泛型连接、依赖工厂、心跳与流量统计 |
| [options](common/options/README.md) | 泛型函数式选项 |
| [pool](common/pool/README.md) | 对象池与缓冲池 |
| [pool/gpool](common/pool/gpool/README.md) | 协程任务池 |
| [profile](common/profile/README.md) | GC 与内存分析服务 |
| [timer](common/timer/README.md) | 最小堆定时器 |

具体 API、示例和注意事项请进入对应模块的 README 查看。

## 网络通信

[network 使用文档](common/network/README.md) 说明了接口与结构关系、生命周期、配置和业务接入方式，并提供可运行的 TCP Server/Client 示例。

- 业务提供消息类型和 Codec，通过 Receiver 接收消息；OnOpen/OnClose 回调按需提供。心跳可通过业务实现并注入 Heartbeator 交给 Conn 托管，也可不注入而由业务自行维护。
- Server 使用依赖工厂，支持共享组件或每连接独立业务会话；Client 管理单次连接，重连由上层控制。
- TCP、KCP、WebSocket 共用消息级接口，支持连接容量限制、应用层心跳和每连接流量统计。


## 开发与测试

```bash
go mod download
go vet ./...
go test ./...
```


## License

本项目基于 [MIT License](LICENSE) 开源。
