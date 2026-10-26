package network

import (
	"context"
	"io"
	"net"
)

// Codec 编解码器接口 实现不应该有内部状态依赖
type Codec[P any] interface {
	// Encode 编码业务包为二进制数据
	Encode(packet P) ([]byte, error)
	// Decode 从 reader 读取并解码一个完整帧
	Decode(reader io.Reader) (packet P, err error)
}

// Transport 对底层 Socket 的兼容封装，是上层唯一依赖的物理 I/O 接口。
type Transport interface {
	// Read 读取数据 不保证同向并发安全
	Read(p []byte) (n int, err error)
	// Write 写入数据 不保证同向并发安全
	Write(p []byte) (n int, err error)
	// Close 关闭连接，可与 Read、Write 并发调用，并使阻塞中的 I/O 退出。
	Close() error
	// LocalAddr 本地地址
	LocalAddr() net.Addr
	// RemoteAddr 远端地址
	RemoteAddr() net.Addr
}

// Dialer 客户端拨号
type Dialer interface {
	// DialContext 拨号
	DialContext(ctx context.Context, address string) (Transport, error)
}

// Listener 服务器监听
type Listener interface {
	//Accept 接受一条连接
	Accept() (Transport, error)
	// Addr 返回监听地址
	Addr() net.Addr
	// 关闭监听器
	Close() error
}

// Connection 连接接口
type Connection[P any] interface {
	// ID 连接标识 进程内唯一
	ID() uint64
	// Run 阻塞运行 重复调用不重复启动循环
	Run()
	// LocalAddr 本地地址
	LocalAddr() net.Addr
	// RemoteAddr 远端地址
	RemoteAddr() net.Addr
	// Send 编码消息并等待入队, 返回成功不表示对端已收到
	Send(context.Context, P) error
	// TrySend 编码消息并尝试入队, 不等待队列空位; 满时返回 ErrSendQueueFull
	TrySend(P) error
	// Close 关闭连接 不等待业务回调退出 不承诺排空发送队列
	Close() error
	// Err 返回首个终止原因，尚未终止时为 nil。
	Err() error
}

// Receiver 网络消息到业务的入站边界
type Receiver[P any] interface {
	// OnMessage 在读循环中顺序调用，应及时返回并响应 context 取消
	// 阻塞回调会延迟后续消息和心跳解析 耗时业务由调用方自行调度
	OnMessage(context.Context, Connection[P], P)
}

// HeartbeatKind 心跳类型
type HeartbeatKind uint8

const (
	HeartbeatKind_Invalid HeartbeatKind = iota // 无效
	HeartbeatKind_Ping                         // 心跳请求
	HeartbeatKind_Pong                         // 心跳响应
)

// Heartbeator 应用层心跳器 实现须保证并发安全且及时返回
type Heartbeator[P any] interface {
	// Ping Ping包
	Ping(seq uint64) P
	// Pong Pong包
	Pong(seq uint64) P
	// Inspect 检查心跳包类型和序列号
	Inspect(packet P) (kind HeartbeatKind, seq uint64, ok bool)
}

// Session 状态枚举
type SessionState uint8

const (
	SessionState_New     SessionState = iota // 创建 尚未启动
	SessionState_Running                     // 已启动
	SessionState_Closed                      // 已发起关闭
)

// SessionManager 规范SessionManager接口 不负责连接的启动和等待结束
type SessionManager[P any] interface {
	// Add 登记连接
	Add(conn Connection[P]) error
	// Remove 注销连接登记 不关闭连接
	Remove(id uint64)
	// Get 获取连接 不保证返回的连接未被并发关闭
	Get(id uint64) (Connection[P], bool)
	// Range 遍历接口
	Range(fn func(Connection[P]) bool)
	// Len 返回当前登记的连接数
	Len() int
	// Close 停止接收登记 并关闭已经登记的连接
	Close() error
}

// ConnDependencies 连接依赖注入集
type ConnDependencies[P any] struct {
	Codec[P]
	Receiver[P]
	Heartbeator[P]
	OnOpen  func(context.Context, Connection[P]) error
	OnClose func(Connection[P], error)
}

// ConnDependenciesFactory 连接依赖构造工厂接口
type ConnDependenciesFactory[P any] func(Transport) (ConnDependencies[P], error)
