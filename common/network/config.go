package network

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"time"
)

// TCPConfig TCP 配置
type TCPConfig struct {
	KeepalivePeriod time.Duration // TCP 内核探测配置选项，不承载业务心跳数据
	NoDelay         bool          // 控制 TCP_NODELAY，开启会关闭 Nagle 算法
	ReadTimeout     time.Duration // Read 超时时间
	WriteTimeout    time.Duration // Write 超时时间
	ReadBufferSize  int           // 读缓存区大小
	WriteBufferSize int           // 写缓存区大小
}

// WsConfig websocket 底层配置
type WsConfig struct {
	MaxMessageSize    int64         // 限制接收的单条 WebSocket 消息载荷大小
	Path              string        // 允许Upgrade的HTTP路径 空值默认为 "/"
	EnableCompression bool          // 是否协商 per-message compression
	HandshakeTimeout  time.Duration // 握手超时时长
	ReadTimeout       time.Duration // Read 超时时间
	WriteTimeout      time.Duration // Write 超时时间
	ReadBufferSize    int           // 读缓存区大小
	WriteBufferSize   int           // 写缓存区大小
	Subprotocols      []string      // 子协议列表
	TLSConfig         *tls.Config   // TLS 配置
}

// WsListenerConfig websocket 服务端配置
type WsListenerConfig struct {
	Address               string                   // 底层tcp监听地址
	MaxPendingConnections int                      // Upgrade完成 等待被Accpet交付的连接限制
	MaxHeaderBytes        int                      // 限制 HTTP Upgrade 请求头，避免在协议升级前消耗无界内存
	ReadHeaderTimeout     time.Duration            // 读取请求头超时时长
	IdleTimeout           time.Duration            // 空闲连接超时时长
	WsConf                WsConfig                 // 底层配置
	CheckOrigin           func(*http.Request) bool // 检查请求 Origin
}

// WsDialerConfig websocket 客户端配置
type WsDialerConfig struct {
	RequireTLS bool                                  // 强制使用 TLS 开启后无协议地址默认使用 wss, 显式 ws 地址返回错误
	WsConf     WsConfig                              // 底层配置
	Header     http.Header                           // 请求头
	Proxy      func(*http.Request) (*url.URL, error) // 代理
}

type KCPEncryption string

const (
	KCPEncryptionNone KCPEncryption = ""
	KCPEncryptionAES  KCPEncryption = "aes"
)

type KCPConfig struct {
	ReadTimeout  time.Duration // Read 超时时间
	WriteTimeout time.Duration // Write 超时时间
	// protocol
	MTU           int  // KCP Session 使用的最大传输单元，必须与链路条件匹配
	SendWindow    int  // KCP 发送窗口大小
	ReceiveWindow int  // KCP 接收窗口大小
	NoDelay       bool // 启用 KCP 低延迟模式
	IntervalMs    int  // KCP 内部更新间隔，单位为毫秒；它不是心跳周期
	FastResend    int  // 触发快速重传所需的重复 ACK 数
	NoCongestion  bool // 禁用 KCP 拥塞窗口控制，可能增加吞吐和网络占用
	AckNoDelay    bool // 允许立即发送 ACK，降低延迟但可能增加包量
	WriteDelay    bool // 决定 Write 后是否延迟 flush，以吞吐换取延迟
	// kcp fec
	DataShards   int // FEC 数据分片数；与 ParityShards 必须同时为零或同时为正
	ParityShards int // FEC 冗余分片数
	// kcp socket
	DSCP            int // UDP socket 的服务质量标记；零值表示不额外设置
	ReadBufferSize  int // 读缓存区大小
	WriteBufferSize int // 写缓存区大小
	// kcp 加密配置
	Mode KCPEncryption // 指定第三方 KCP 适配器使用的块变换类型
	Key  []byte        // 块加密密钥；配置复制时必须深拷贝，不能由调用方继续修改
}

// ConnConfig 连接运行配置。
type ConnConfig struct {
	SendQueueSize     int           // 编码后字节的发送队列容量，<=0 使用默认值
	ReadBufferSize    int           // 接收缓冲初始容量，<=0 使用默认值
	HeartbeatInterval time.Duration // 主动 Ping 和超时检查间隔，<=0 只响应心跳、不主动探测
	HeartbeatTimeout  time.Duration // 无入站数据的最大时长；主动心跳启用时 <=0 默认三倍间隔
}
