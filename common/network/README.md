# network

定义网络组件的公共接口，并提供连接接受退避和全局流量统计。具体 LTV 实现见 [ltv/README.md](ltv/README.md)。

## 接口

| 接口 | 职责 |
| --- | --- |
| `IProtocolCoder` | 数据包 Encode/Decode 和协议头大小 |
| `IPacket` | 数据、长度、头部和总长度 |
| `IConnection` | 连接生命周期、地址以及同步/队列发送 |
| `IConnectionManager` | 连接增删、查找、计数和遍历 |
| `IServer` | 服务端生命周期、回调、协议和定时器 |
| `IClient` | 客户端生命周期、重连、回调和 URL |
| `IHandler` | 执行一类消息 |
| `IDispatcher` | 注册、分发、暂停和恢复消息处理器 |

这些接口用于隔离协议、传输和业务处理。实现自定义协议时，通常需要实现 `IPacket` 与 `IProtocolCoder`；实现自定义传输时再实现连接、客户端或服务端接口。

## 流量统计

包级变量 `network.TS` 是默认的 `TrafficStatistics`：

```go
network.TS.IncrRead(uint32(len(data)))
network.TS.IncrWrite(uint32(len(data)))

readCount, writeCount, readBytes, writeBytes := network.TS.Get()
network.TS.Reset()
```

计数器使用原子操作，可由多个连接并发更新。

## AcceptDelay

`AcceptDelay` 在监听器连续接受连接失败时提供递增退避，并在成功后通过 `Reset` 恢复。LTV 服务端已在内部使用，业务通常无需直接调用。
