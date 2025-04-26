package network

import "net"

// IProtocolCoder 网络协议编码器
type IProtocolCoder interface {
	GetHeaderSize() int
	// Decode 解析缓冲区数据
	// 返回解析的数据，解析的长度，可能的错误
	Decode(buffer []byte) (IPacket, uint32, error)
	// Encode 编码数据
	// 返回编码后的数据，可能的错误
	Encode(data IPacket) ([]byte, error)
}

// IPacket 网络数据包
type IPacket interface {
	// GetData 获取数据域
	GetData() []byte
	// GetLength 获取数据域长度
	GetLength() uint32
	// GetHeader 获取头部域
	GetHeader() any
	// GetTotalLength 获取数据包总长度
	GetTotalLength() uint32
}

// IConnection 网络连接
type IConnection interface {
	// Start 启动连接
	Start()
	// Close 关闭连接
	Close() error
	// GetConnectionID 获取连接ID
	GetConnectionID() uint64
	// RemoteAddr 获取远程地址
	RemoteAddr() net.Addr
	// LocalAddr 获取本地地址
	LocalAddr() net.Addr
	// RemoteAddrString 获取远程地址字符串
	RemoteAddrString() string
	// LocalAddrString 获取本地地址字符串
	LocalAddrString() string
	// IsAlive 判断当前连接是否存活
	IsAlive() bool
}

// IConnectionManager 连接管理器
type IConnectionManager interface {
	// Count 连接数量
	Count() int
}

type IServer interface {
	// Serve 启动服务
	Serve()
	// Close 关闭服务
	Close() error
	// SetOnConnect 设置连接回调
	SetOnConnect(fn func(conn IConnection))
	// OnConnect 获取连接回调
	OnConnect() func(conn IConnection)
	// SetOnDisconnect 设置断开连接回调
	SetOnDisconnect(fn func(conn IConnection))
	// OnDisconnect 获取断开连接回调
	OnDisconnect() func(conn IConnection)
	// ProtocolCoder 获取协议编码器
	ProtocolCoder() IProtocolCoder
	// HeartbeatFunc 获取心跳回调
	HeartbeatFunc() func(conn IConnection)
	// SetHeartbeatFunc 设置心跳回调
	SetHeartbeatFunc(fn func(conn IConnection))
	// GetDispatchMsg 获取消息分发
	GetDispatchMsg() func(packet IPacket)
}
