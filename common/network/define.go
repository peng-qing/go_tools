package network

const (
	defaultTCPReadBufferSize = 32 * 1024 // 默认 TCP 读缓冲区大小

	defaultMaxWsMessageSize        = 1 * 1024 * 1024 // 默认 WebSocket 最大消息大小 1MB
	defaultMaxWsPendingConnections = 1024            // 默认最大等待连接数
	defaultMaxWsConcurrentUpgrades = 256             // 默认最大并发升级数

	defaultKCPReadBufferSize = 32 * 1024 // 默认 KCP 读缓冲区大小
)
