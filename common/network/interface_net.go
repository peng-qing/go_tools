package network

import (
	"net"
	"net/url"
)

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
	// Send 立刻发送数据
	Send(data []byte) error
	// SendToQueue 发送数据到队列
	SendToQueue(data []byte) error
}

// IConnectionManager 连接管理器
type IConnectionManager interface {
	// Add 添加连接
	Add(conn IConnection)
	// Remove 移除连接
	Remove(conn IConnection)
	// RemoveByConnectionID 根据连接ID移除连接
	RemoveByConnectionID(connID uint64)
	// Get 获取连接
	Get(connID uint64) IConnection
	// Count 连接数量
	Count() int
	// GetAllConnID 获取所有连接
	GetAllConnID() []uint64
	// Range 遍历连接
	Range(fn func(connId uint64, conn IConnection) error) error
}

// IServer 服务接口
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
	GetDispatchMsg() func(conn IConnection, packet IPacket)
	// SetDispatchMsg 设置消息分发
	SetDispatchMsg(fn func(conn IConnection, packet IPacket))
	// GetConnectionManager 获取连接管理器
	GetConnectionManager() IConnectionManager
}

// IClient 客户端接口
type IClient interface {
	// Start 启动客户端
	Start()
	// Close 关闭客户端
	Close() error
	// Restart 重新启动客户端
	Restart()
	// Connection 获取连接
	Connection() IConnection
	// SetConnection 设置连接
	SetConnection(conn IConnection)
	// OnConnect 获取连接回调
	OnConnect() func(conn IConnection)
	// SetOnConnect 设置连接回调
	SetOnConnect(fn func(conn IConnection))
	// OnDisconnect 获取断开连接回调
	OnDisconnect() func(conn IConnection)
	// SetOnDisconnect 设置断开连接回调
	SetOnDisconnect(fn func(conn IConnection))
	// ProtocolCoder 获取协议编码器
	ProtocolCoder() IProtocolCoder
	// HeartbeatFunc 获取心跳函数
	HeartbeatFunc() func(conn IConnection)
	// SetHeartbeatFunc 设置心跳函数
	SetHeartbeatFunc(fn func(conn IConnection))
	// GetDispatchMsg 获取消息分发函数
	GetDispatchMsg() func(conn IConnection, packet IPacket)
	// SetDispatchMsg 设置消息分发函数
	SetDispatchMsg(fn func(conn IConnection, packet IPacket))
	// GetUrl 获取URL
	GetUrl() *url.URL
	// SetUrl 设置URL
	SetUrl(url *url.URL)
}

// IExecutor 执行器接口 暂时定义为一个空接口
type IExecutor interface {
}

// IHandler 命令接口
type IHandler interface {
	Execute(executor IExecutor, packet IPacket)
}

// IDispatcher 消息分发器
type IDispatcher interface {
	// Register 注册消息
	Register(id uint32, handler IHandler)
	// DispatchMsg 分发消息
	DispatchMsg(conn IConnection, packet IPacket)
	// Pause 暂停消息
	Pause(id uint32)
	// Resume 恢复消息
	Resume(id uint32)
}
