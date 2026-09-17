package network

import (
	"net"
	"net/http"
	"time"
)

// IOConfig 是输入输出通用配置
type IOConfig struct {
	ReadTimeout  time.Duration // 单次 Read 操作的超时时时间
	WriteTimeout time.Duration // 单次 Write 操作的超时时时间
}

// =========================================================
// Tcp Configs
// =========================================================

// TcpTransportConfig 是 TCP 传输层配置
type TcpTransportConfig struct {
	ReadBufferSize int                  // 单次 Read 分配的接收缓冲区大小
	IO             *IOConfig            // 输入输出配置
	KeepAlive      *net.KeepAliveConfig // 内核 TCP 探测配置，不承载业务心跳数据
	NoDelay        bool                 // 是否关闭 Nagle 算法
}

// =========================================================
// Websocket Configs
// =========================================================

// WebsocketHandshakeConfig 是 WebSocket 握手配置
type WebsocketHandshakeConfig struct {
	Timeout           time.Duration // ws 握手的最长等待时间
	EnableCompression bool          // 是否协商 per-message compression；它只影响新握手。
}

// WebsocketTransportConfig 是 WebSocket 传输层配置
type WebsocketTransportConfig struct {
	IO              *IOConfig                 // 输入输出配置
	ReadBufferSize  int                       // ws 握手后使用的读缓冲区大小
	WriteBufferSize int                       // ws 握手后使用的写缓冲区大小
	MaxMessageSize  int64                     // 单个 WebSocket Message 允许读取的最大字节数
	Handshake       *WebsocketHandshakeConfig // 握手配置
}

// WebsocketListenerConfig 是 WebSocket 监听器配置
type WebsocketListenerConfig struct {
	Address               string                    // websocket 监听地址
	Path                  string                    // 允许 Upgrade 的 HTTP 路径，空值会归一化为 "/"
	Transport             *WebsocketTransportConfig // 传输层配置
	MaxPendingConnections int                       // Upgrade 完成、尚未被 Accept 取走的连接上限
	MaxConcurrentUpgrades int                       // 同时执行 WebSocket Upgrade 的请求上限
	MaxHeaderBytes        int                       // 限制 HTTP Upgrade 请求头，避免在协议升级前消耗无界内存。
	CheckOrigin           func(*http.Request) bool  // 检查请求 Origin
}

// =========================================================
// Kcp Configs
// =========================================================

// KCPEncryption 标识第三方 KCP 适配器使用的块加密方式
type KCPEncryption string

const (
	KCPEncryptionNone KCPEncryption = ""
	KCPEncryptionAES  KCPEncryption = "aes"
)

// KcpProtocolConfig 是 KCP 协议配置
type KcpProtocolConfig struct {
	MTU           int   // KCP Session 使用的最大传输单元，必须与链路条件匹配
	SendWindow    int   // Kcp 发送窗口大小
	ReceiveWindow int   // Kcp 接收窗口大小
	NoDelay       bool  // 启用 KCP 低延迟模式
	Interval      int64 // KCP 内部更新间隔，单位为毫秒；它不是心跳周期
	FastResend    int   // 触发快速重传所需的重复 ACK 数
	NoCongestion  bool  // 禁用 KCP 拥塞窗口控制，可能增加吞吐和网络占用
	AckNoDelay    bool  // 允许立即发送 ACK，降低延迟但可能增加包量
	WriteDelay    bool  // 决定 Write 后是否延迟 flush，以吞吐换取延迟
}

// KcpFECConfig 是 KCP FEC 配置
type KcpFECConfig struct {
	DataShards   int // FEC 数据分片数；与 ParityShards 必须同时为零或同时为正
	ParityShards int // FEC 冗余分片数
}

// KcpSocketConfig 是 KCP Socket 配置
type KcpSocketConfig struct {
	DSCP              int // UDP socket 的服务质量标记；零值表示不额外设置
	SocketReadBuffer  int // 底层 UDP socket 接收缓冲区大小；零值表示不额外设置
	SocketWriteBuffer int // 底层 UDP socket 发送缓冲区大小；零值表示不额外设置
}

// KcpEncryptionConfig 是 KCP 加密配置
type KcpEncryptionConfig struct {
	Mode KCPEncryption // 指定第三方 KCP 适配器使用的块变换类型
	Key  []byte        // 块加密密钥；配置复制时必须深拷贝，不能由调用方继续修改
}

// KcpTransportConfig 是 KCP 传输层配置
type KcpTransportConfig struct {
	IO             *IOConfig           // 输入输出配置
	ReadBufferSize int                 // 单次 Read 分配的接收缓冲区大小
	Protocol       KcpProtocolConfig   // KCP 协议配置
	FEC            KcpFECConfig        // KCP FEC 配置
	Socket         KcpSocketConfig     // KCP Socket 配置
	Encryption     KcpEncryptionConfig // KCP 加密配置
}

// =========================================================
// Session Configs
// =========================================================

// ReceiverErrorPolicy 是接收器错误策略
type ReceiverErrorPolicy uint8

const (
	// 关闭连接
	ReceiverErrorPolicyClose ReceiverErrorPolicy = iota
	// 继续接收
	ReceiverErrorPolicyContinue
)

// Session 依赖配置
type SessionConfig struct {
	SendQueueSize        int                 // 发送队列消息数量
	MaxEncodedPacketSize int                 // 单个完整编码帧最大大小(包含协议头)
	MaxReceiveBuffer     int                 // 最大接收缓冲区大小
	MaxPacketsPerRead    int                 // 限制流模式单批投递数量 达到后让出调度再处理已有缓存
	ReceiverErrorPolicy  ReceiverErrorPolicy // 接收器错误策略
}
