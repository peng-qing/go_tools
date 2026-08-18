# ltv

Length-Type-Value（LTV）网络协议实现，支持 TCP 和 WebSocket 客户端/服务端。

## 数据包格式

`LTVHeader` 包含：

- `Length uint32`：数据体长度。
- `Type uint32`：业务消息类型。

`LTVPacket` 在头部之后携带字节数据。使用 `NewLTVPacket(msgType, data)` 创建包；`LTVProtocolCoder` 根据配置使用大端或小端编码。

## 传输模式

| 常量 | 服务端 | 客户端 |
| --- | --- | --- |
| `NetMode_Default` | 同时监听 TCP 和 WebSocket | 默认使用 TCP |
| `NetMode_Tcp` | TCP | TCP |
| `NetMode_Websocket` | WebSocket | WebSocket |

## 配置

`LTVConnectionConfig` 中的时间字段单位均为毫秒：

| 字段 | 说明 |
| --- | --- |
| `Heartbeat` | 心跳执行间隔 |
| `MaxHeartbeat` | 最大无心跳间隔，超过后视为超时 |
| `ReadTimeout` | 单次读取超时 |
| `WriteTimeout` | 单次写入超时 |
| `MaxIOReadSize` | 单次最大读取字节数 |
| `SendQueueSize` | 异步发送队列容量 |

服务端额外配置监听 IP、TCP 端口、WebSocket 端口与路径、最大连接数、定时器队列大小和毫秒级调度频率。客户端配置目标 IP、端口、模式和连接参数。两端的字节序必须一致，且 `Connection` 不能为空。

## 服务端示例

```go
package main

import (
    "fmt"

    "github.com/peng-qing/go_tools/common/network"
    "github.com/peng-qing/go_tools/common/network/ltv"
)

func main() {
    server := ltv.NewLTVServer(1, &ltv.LTVServerConfig{
        IP:               "127.0.0.1",
        Port:             30101,
        Mode:             int(ltv.NetMode_Tcp),
        MaxConn:          100,
        UsedLittleEndian: true,
        TimerQueueSize:   1024,
        Frequency:        100,
        Connection: &ltv.LTVConnectionConfig{
            Heartbeat:     5000,
            MaxHeartbeat:  10000,
            ReadTimeout:   1000,
            WriteTimeout:  1000,
            MaxIOReadSize: 1024 * 1024,
            SendQueueSize: 100,
        },
    })

    server.SetOnConnect(func(conn network.IConnection) {
        fmt.Println("connected:", conn.GetConnectionID())
    })
    server.SetDispatchMsg(func(conn network.IConnection, packet network.IPacket) {
        fmt.Println("message:", packet.GetHeader(), string(packet.GetData()))
    })

    server.Serve()
    defer server.Close()
}
```

## 客户端与发送

客户端通过 `NewLTVClient` 创建，使用同结构的 `LTVClientConfig`。注册 `SetOnConnect`、`SetOnDisconnect`、`SetHeartbeatFunc` 和 `SetDispatchMsg` 后调用 `Start`。

```go
packet := ltv.NewLTVPacket(1001, []byte("hello"))
err := conn.SendPacketToQueue(packet)
```

`Send`/`SendPacket` 立即写入连接；`SendToQueue`/`SendPacketToQueue` 放入发送队列。使用完成后调用 `Close`，客户端需要重新连接时可调用 `Restart`。

## 消息分发

`LTVDispatcher` 按消息类型注册 `network.IHandler`，并支持 `Pause(id)` 与 `Resume(id)`。构造参数用于限定可接受的消息 ID 范围及处理超时阈值。

## 文件分工

| 文件 | 说明 |
| --- | --- |
| `ltv_protocol_coder.go`、`ltv_packet.go` | 协议编解码与数据包 |
| `ltv_tcp_connection.go` | TCP 连接 |
| `ltv_websocket_connection.go` | WebSocket 连接 |
| `ltv_server.go`、`ltv_client.go` | 服务端与客户端生命周期 |
| `ltv_connection_manager.go` | 连接管理 |
| `ltv_dispatcher.go` | 消息处理器注册和分发 |
| `ltv_config.go`、`define.go` | 配置与枚举 |

