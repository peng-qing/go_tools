package network

import (
	"context"
	"net"
)

// Codec 是网络协议的编解码器，用于将数据包转换为字节流和从字节流转换为数据包。
type Codec[P any] interface {
	// Decode 从data前缀解析出一个Packet，并返回解析出的字节数和错误。
	Decode(data []byte) (packet P, consumed int, err error)
	// Encode 将一个Packet编码为字节流
	Encode(packet P) ([]byte, error)
}

// Transport 统一不同协议的阻塞读写 明确边界和关闭语义
type Transport interface {
	// Read 读取数据 返回读取的数据和错误
	Read() ([]byte, error)
	// Write 写入数据 返回写入的数据和错误
	Write(data []byte) error
	// Close 关闭Transport
	Close() error
	// LocalAddr 返回Transport的本地地址
	LocalAddr() net.Addr
	// RemoteAddr 返回Transport的远程地址
	RemoteAddr() net.Addr
}

// Listener 监听器 用于服务端
type Listener interface {
	// Accept 接受并返回一个Transport 返回Transport时必须已经达到可供Session使用的状态
	Accept() (Transport, error)
	// Addr 返回Listener的地址
	ListenerAddr() net.Addr
	// Close 关闭Listener 必须解除阻塞的Accept 需要避免一个对端的可控输入直接终止整个服务
	Close() error
}

// Dialer 拨号器 用户客户端
type Dialer interface {
	// Dial 建立一个客户端性质的Transport
	// 成功后 Transport 先交给调用方，Session 构造成功后才接管
	// 失败时 Dialer 必须关闭已经创建的 socket 和临时资源
	Dial(ctx context.Context, address string) (Transport, error)
}

// Connection 连接 用于表示一个网络连接 是业务可以长期持有的会话能力
// 不对外暴露底层Session,Transport,Codec,内部goroutine,只提供发送,查询状态和关闭连接的能力
// 不会绕过队列直接并发写底层socket, 不依赖某个协议的具体连接类型
type Connection[P any] interface {
	// ID 返回当前连接的框架标识 只保证所属Server或者Client生命周期内唯一
	// 重连会新分配ID，不同业务可以产生相同ID
	ID() uint64
	// LocalAddr 是建连时保存的地址快照
	LocalAddr() net.Addr
	// RemoteAddr 是建连时保存的地址快照
	RemoteAddr() net.Addr
	// Send 编码 packet，并允许调用方通过 ctx 限制等待发送队列的时间
	Send(ctx context.Context, packet P) error
	// TrySend 不等待队列；队列已满时返回 ErrSendQueueFull。
	TrySend(packet P) error
	// Done 在连接资源和生命周期回调完成后关闭
	Done() <-chan struct{}
	// Err 返回触发关闭的第一原因；主动 Close 通常返回 nil。
	Err() error
	// Close 幂等地发起关闭，它不能代替等待 Done。
	Close() error
}

// Receiver 接收 Codec 解出的完整消息，是 Network 核心与业务处理的唯一入站边界
// Session 在单条连接的 readLoop 中同步调用 OnMessage，因此同一连接的消息天然保持解码顺序
// Receiver 也不需要为了同一连接实现并发安全
// 优点是顺序和错误归属明确
// 问题是耗时业务会直接阻塞该连接后续读取
// 需要并行处理时，Receiver 应把消息投递到业务自己管理的有界 mailbox，不能在这里启动无界 goroutine
type Receiver[P any] interface {
	// OnMessage 在单条连接的 readLoop 中同步调用，因此同一连接的消息天然保持解码顺序
	// Receiver 也不需要为了同一连接实现并发安全
	OnMessage(ctx context.Context, conn Connection[P], packet P) error
}

// Observer 接收网络观测事件 不是异步事件总线
// 所有方法都由网络路径同步调用 必须快速返回,不得阻塞,操作当前连接,不能panic
// 适合递增指标或者轻量事件投递 不适合执行IO或者业务状态修改
type Observer interface {
	// OnOpen 在连接建立时调用
	OnOpen(id uint64, local, remote net.Addr)
	// OnRead 在读取数据时调用
	OnRead(id uint64, bytes int)
	// OnWrite 在写入数据时调用
	OnWrite(id uint64, bytes int)
	// OnDecodeError 在解码数据时发生错误时调用
	OnDecodeError(id uint64, err error)
	// OnReceiverError 在接收数据时发生错误时调用
	OnReceiverError(id uint64, err error)
	// OnClose 在连接关闭时调用
	OnClose(id uint64, err error)
}

// SessionLifecycle 只保留业务真正需要的连接可用与最终关闭通知
type SessionLifecycle[P any] interface {
	// OnReady 调用时 Session 已加入 Manager、Send 可用，但首包尚未投递给 Receiver
	OnReady(ctx context.Context, conn Connection[P])
	// OnClosed 调用时 Session 已从 Manager 摘除
	// Transport 已关闭，I/O goroutine 已退出且 Session 接收缓存已释放
	// 它返回后 Done 关闭
	OnClosed(conn Connection[P], cause error)
}
