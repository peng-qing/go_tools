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
| [network](common/network/README.md) | 网络抽象接口与统计 |
| [network/ltv](common/network/ltv/README.md) | LTV TCP/WebSocket 实现 |
| [options](common/options/README.md) | 泛型函数式选项 |
| [pool](common/pool/README.md) | 对象池与缓冲池 |
| [pool/gpool](common/pool/gpool/README.md) | 协程任务池 |
| [profile](common/profile/README.md) | GC 与内存分析服务 |
| [timer](common/timer/README.md) | 最小堆定时器 |

具体 API、示例和注意事项请进入对应模块的 README 查看。

## 开发与测试

```bash
go mod download
go vet ./...
go test ./...
```


## License

本项目基于 [MIT License](LICENSE) 开源。
