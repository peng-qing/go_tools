package ltv

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"go_tools/common/network"

	"github.com/gorilla/websocket"
)

var (
	// LTVWebsocketConnection 实现了 network.IConnection
	_ network.IConnection = (*LTVWebsocketConnection)(nil)
)

// LTVWebsocketConnection websocket连接
type LTVWebsocketConnection struct {
	connID         uint64                                                 // 连接ID 客户端性质连接默认 connID 为0
	rwc            *websocket.Conn                                        // 原始连接
	connM          network.IConnectionManager                             // 连接管理 客户端性质连接默认不存在
	msgSendChan    chan []byte                                            // 等待发送消息队列
	side           NetSide                                                // 连接类型
	ctx            context.Context                                        // 上下文
	ctxCancel      context.CancelFunc                                     // 上下文取消
	onConnect      func(conn network.IConnection)                         // 连接建立回调
	onDisconnect   func(conn network.IConnection)                         // 连接断开回调
	protocolCoder  network.IProtocolCoder                                 // 协议编解码器
	heartbeatFunc  func(conn network.IConnection)                         // 自定义心跳函数
	dispatchFunc   func(conn network.IConnection, packet network.IPacket) // 自定义消息分发函数
	connConf       *LTVConnectionConfig                                   // 连接配置
	lastActiveTime time.Time                                              // 最后活动时间
	lock           sync.Mutex                                             // 锁
	wg             sync.WaitGroup                                         // 等待组
}

// ===============================================================================
// 实现 network.IConnection
// ===============================================================================

// Start 连接启动
func (ltv *LTVWebsocketConnection) Start() {
	ltv.ctx, ltv.ctxCancel = context.WithCancel(context.Background())
	// 启动主循环
	go ltv.run()

	ltv.callOnConnect()
}

// Close 关闭连接
func (ltv *LTVWebsocketConnection) Close() error {
	if ltv.isClosed() {
		return errors.New("connection is closed")
	}
	if ltv.ctx != nil && ltv.ctxCancel != nil {
		ltv.ctxCancel()
	}
	// 执行回调
	ltv.callOnDisconnect()

	if err := ltv.rwc.Close(); err != nil {
		slog.Error("[LTVTCPConnection] close rwc conn failed", "connID", ltv.connID, "err", err)
	}
	// 移除连接
	if ltv.side == NodeSide_Server && ltv.connM != nil {
		ltv.connM.RemoveByConnectionID(ltv.connID)
	}

	return nil
}

// GetConnectionID 获取连接ID
func (ltv *LTVWebsocketConnection) GetConnectionID() uint64 {
	return ltv.connID
}

// RemoteAddr 获取远程地址
func (ltv *LTVWebsocketConnection) RemoteAddr() net.Addr {
	return ltv.rwc.RemoteAddr()
}

// LocalAddr 获取本地地址
func (ltv *LTVWebsocketConnection) LocalAddr() net.Addr {
	return ltv.rwc.LocalAddr()
}

// RemoteAddrString 获取远程地址字符串
func (ltv *LTVWebsocketConnection) RemoteAddrString() string {
	return ltv.rwc.RemoteAddr().String()
}

// LocalAddrString 获取本地地址字符串
func (ltv *LTVWebsocketConnection) LocalAddrString() string {
	return ltv.rwc.LocalAddr().String()
}

// IsAlive 判断连接是否存活
func (ltv *LTVWebsocketConnection) IsAlive() bool {
	if ltv.isClosed() {
		return false
	}
	return time.Now().Sub(ltv.lastActiveTime) <= time.Duration(ltv.connConf.MaxHeartbeat)*time.Millisecond
}

// Send 发送数据
func (ltv *LTVWebsocketConnection) Send(data []byte) error {
	if ltv.isClosed() {
		slog.Error("[LTVWebsocketConnection] Send conn is closed", "connID", ltv.connID)
		return errors.New("connection is closed")
	}
	if len(data) <= 0 {
		slog.Error("[LTVWebsocketConnection] Send data is empty", "connID", ltv.connID)
		return errors.New("pack data is empty")
	}
	ltv.lock.Lock()
	defer ltv.lock.Unlock()

	// 设置写超时
	if ltv.connConf.WriteTimeout > 0 {
		_ = ltv.rwc.SetWriteDeadline(time.Now().Add(time.Duration(ltv.connConf.WriteTimeout) * time.Millisecond))
	}
	// 发送数据
	if err := ltv.rwc.WriteMessage(websocket.BinaryMessage, data); err != nil {
		slog.Error("[LTVWebsocketConnection] write loop write error", "connID", ltv.connID, "err", err)
		if err = ltv.Close(); err != nil {
			slog.Error("[LTVWebsocketConnection] write loop close conn error", "connID", ltv.connID, "err", err)
		}
		return err
	}
	// 取消写超时
	if ltv.connConf.WriteTimeout > 0 {
		_ = ltv.rwc.SetWriteDeadline(time.Time{})
	}

	// 更新网络统计信息
	network.TS.IncrWrite(uint32(len(data)))

	return nil
}

// SendToQueue 发送数据到队列中
func (ltv *LTVWebsocketConnection) SendToQueue(data []byte) error {
	if ltv.isClosed() {
		slog.Error("[LTVWebsocketConnection] SendToQueue conn is closed", "connID", ltv.connID)
		return errors.New("connection is closed")
	}
	if len(data) <= 0 {
		slog.Error("[LTVWebsocketConnection] SendToQueue data is empty", "connID", ltv.connID)
		return errors.New("pack data is empty")
	}
	select {
	case ltv.msgSendChan <- data:
		return nil
	case <-ltv.ctx.Done():
		return errors.New("connection closed when send buff msg")
	}
}

// SendPacket 发送数据包
func (ltv *LTVWebsocketConnection) SendPacket(packet network.IPacket) error {
	if ltv.isClosed() {
		slog.Error("[LTVWebsocketConnection] SendPacket conn is closed", "connID", ltv.connID)
		return errors.New("connection is closed")
	}
	if packet == nil {
		slog.Error("[LTVWebsocketConnection] SendPacket packet is nil", "connID", ltv.connID)
		return errors.New("packet is nil")
	}
	data, err := ltv.protocolCoder.Encode(packet)
	if err != nil {
		slog.Error("[LTVWebsocketConnection] SendPacket Encode failed", "connID", ltv.connID, "err", err)
		return err
	}
	// 发送数据
	return ltv.Send(data)
}

// SendPacketToQueue 添加数据包到队列
func (ltv *LTVWebsocketConnection) SendPacketToQueue(packet network.IPacket) error {
	if ltv.isClosed() {
		slog.Error("[LTVWebsocketConnection] SendPacketToQueue conn is closed", "connID", ltv.connID)
		return errors.New("connection is closed")
	}
	if packet == nil {
		slog.Error("[LTVWebsocketConnection] SendPacketToQueue packet is nil", "connID", ltv.connID)
		return errors.New("packet is nil")
	}
	data, err := ltv.protocolCoder.Encode(packet)
	if err != nil {
		slog.Error("[LTVWebsocketConnection] SendPacketToQueue Encode failed", "connID", ltv.connID, "err", err)
		return err
	}
	return ltv.SendToQueue(data)
}

// ===============================================================================
// 私有接口
// ===============================================================================

// callOnConnect 连接建立回调
func (ltv *LTVWebsocketConnection) callOnConnect() {
	if ltv.onConnect != nil {
		ltv.onConnect(ltv)
	}
}

// callOnDisconnect 连接建立回调
func (ltv *LTVWebsocketConnection) callOnDisconnect() {
	if ltv.onDisconnect != nil {
		ltv.onDisconnect(ltv)
	}
}

// isClosed 判断连接是否关闭
func (ltv *LTVWebsocketConnection) isClosed() bool {
	if ltv.ctx == nil || ltv.ctx.Err() != nil {
		return true
	}
	return false
}

// run 连接主循环
func (ltv *LTVWebsocketConnection) run() {
	// 更新活跃时间
	ltv.updateLastActiveTime()
	// 启动读写循环
	ltv.wg.Add(1)
	go func() {
		defer ltv.wg.Done()
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[LTVWebsocketConnection] run read loop panic", "err", err, "stack", debug.Stack())
			}
		}()
		ltv.readLoop()
	}()

	ltv.wg.Add(1)
	go func() {
		defer ltv.wg.Done()
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[LTVWebsocketConnection] run write loop panic", "err", err, "stack", debug.Stack())
			}
		}()
		ltv.writeLoop()
	}()

	ltv.wg.Add(1)
	go func() {
		defer ltv.wg.Done()
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[LTVWebsocketConnection] run heartbeat loop panic", "err", err, "stack", debug.Stack())
			}
		}()
		ltv.keepalive()
	}()

	// 监听关闭信号
	select {
	case <-ltv.ctx.Done():
		ltv.finalizer()
		return
	}
}

// updateLastActiveTime 更新活跃时间
func (ltv *LTVWebsocketConnection) updateLastActiveTime() {
	ltv.lastActiveTime = time.Now()
}

// readLoop 读循环
func (ltv *LTVWebsocketConnection) readLoop() {
	for {
		select {
		case <-ltv.ctx.Done():
			slog.Info("[LTVWebsocketConnection] read loop conn close", "connID", ltv.connID)
			return
		default:
			if ltv.isClosed() {
				slog.Info("[LTVWebsocketConnection] read loop conn is closed", "connID", ltv.connID)
				return
			}
			if ltv.connConf.ReadTimeout > 0 {
				_ = ltv.rwc.SetReadDeadline(time.Now().Add(time.Duration(ltv.connConf.ReadTimeout) * time.Millisecond))
			}
			msgType, buffer, err := ltv.rwc.ReadMessage()
			if err != nil {
				if err = ltv.Close(); err != nil {
					slog.Error("[LTVWebsocketConnection] read loop close conn error", "connID", ltv.connID, "err", err)
				}
				return
			}
			if ltv.connConf.ReadTimeout > 0 {
				_ = ltv.rwc.SetReadDeadline(time.Time{})
			}
			// ping/pong
			if msgType == websocket.PingMessage {
				ltv.updateLastActiveTime()
				continue
			}
			// 正常读取到对端数据
			if n := len(buffer); n > 0 {
				// 读取到非ping/pong 是否更新活跃时间...
				packet, totalLen, decodeErr := ltv.protocolCoder.Decode(buffer)
				if decodeErr != nil {
					// 解包失败 关闭连接
					slog.Error("[LTVWebsocketConnection] readLoop protocol coder decode  message failed", "connID", ltv.connID, "readLength", n, "err", err)
					if err = ltv.Close(); err != nil {
						slog.Error("[LTVWebsocketConnection] read loop close conn error", "connID", ltv.connID, "err", err)
					}
					return
				}
				if packet == nil && totalLen == 0 {
					// 数据不足
					slog.Error("[LTVWebsocketConnection] readLoop protocol coder decode message data empty", "connID", ltv.connID)
					continue
				}
				if packet != nil && totalLen > 0 {
					// 处理完整数据包
					// packet -> dispatcher -> handler -> message -> logic
					ltv.dispatchFunc(ltv, packet)
				}
			}
		}
	}
}

// writeLoop 写循环
func (ltv *LTVWebsocketConnection) writeLoop() {
	for {
		select {
		case <-ltv.ctx.Done():
			slog.Info("[LTVWebsocketConnection] write loop conn close", "connID", ltv.connID)
			close(ltv.msgSendChan)
			return
		case msg := <-ltv.msgSendChan:
			if ltv.isClosed() {
				slog.Info("[LTVWebsocketConnection] write loop conn is closed", "connID", ltv.connID)
				return
			}
			if err := ltv.Send(msg); err != nil {
				slog.Error("[LTVWebsocketConnection] write loop write error", "connID", ltv.connID, "err", err)
				if err = ltv.Close(); err != nil {
					slog.Error("[LTVWebsocketConnection] write loop close conn error", "connID", ltv.connID, "err", err)
				}
				return
			}
		}
	}
}

// keepalive 心跳循环
func (ltv *LTVWebsocketConnection) keepalive() {
	ticker := time.NewTicker(time.Duration(ltv.connConf.Heartbeat) * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ltv.ctx.Done():
			slog.Info("[LTVWebsocketConnection] keepalive conn close", "connID", ltv.connID)
			return
		case <-ticker.C:
			if ltv.isClosed() {
				slog.Info("[LTVWebsocketConnection] keepalive conn is closed", "connID", ltv.connID)
				return
			}
			if !ltv.IsAlive() {
				slog.Warn("[LTVWebsocketConnection] keepalive not alive", "connID", ltv.connID, "lastActivity", ltv.lastActiveTime.UnixNano())
				if err := ltv.Close(); err != nil {
					slog.Error("[LTVWebsocketConnection] keepalive close conn error", "connID", ltv.connID, "err", err)
				}
				return
			}
			if ltv.heartbeatFunc != nil {
				ltv.heartbeatFunc(ltv)
			}
		}
	}
}

// finalizer 销毁
func (ltv *LTVWebsocketConnection) finalizer() {
	// 等待相关子协程退出
	ltv.wg.Wait()

	// 执行回调
	ltv.callOnDisconnect()
	// 关闭连接
	if err := ltv.rwc.Close(); err != nil {
		slog.Error("[LTVWebsocketConnection] Close rwc conn failed", "connID", ltv.connID, "err", err)
	}

	// 移除连接管理
	if ltv.side == NodeSide_Server && ltv.connM != nil {
		ltv.connM.RemoveByConnectionID(ltv.connID)
	}

	slog.Info("[LTVWebsocketConnection] finalizer success", "connID", ltv.connID, "side", ltv.side)
}

// ===============================================================================
// 实例化接口
// ===============================================================================

// newLTVServerWebsocketConnection 创建LTV websocket连接 服务器
func newLTVServerWebsocketConnection(connID uint64, conn *websocket.Conn, server network.IServer, connConf *LTVConnectionConfig) *LTVWebsocketConnection {
	instance := &LTVWebsocketConnection{
		connID:        connID,
		rwc:           conn,
		side:          NodeSide_Server,
		connConf:      connConf,
		msgSendChan:   make(chan []byte, connConf.SendQueueSize),
		connM:         server.GetConnectionManager(),
		heartbeatFunc: server.HeartbeatFunc(),
		onConnect:     server.OnConnect(),
		onDisconnect:  server.OnDisconnect(),
		protocolCoder: server.ProtocolCoder(),
		dispatchFunc:  server.GetDispatchMsg(),
		lock:          sync.Mutex{},
		wg:            sync.WaitGroup{},
	}

	// 注册到连接管理器
	instance.connM.Add(instance)

	return instance
}

// NewLTVClientConnection 创建LTV Websocket连接 客户端
func newLTVClientWebsocketConnection(client network.IClient, conn *websocket.Conn, connConf *LTVConnectionConfig) *LTVWebsocketConnection {
	instance := &LTVWebsocketConnection{
		connID:        0, // 客户端忽略
		rwc:           conn,
		msgSendChan:   make(chan []byte, connConf.SendQueueSize),
		side:          NodeSide_Client,
		onConnect:     client.OnConnect(),
		onDisconnect:  client.OnDisconnect(),
		protocolCoder: client.ProtocolCoder(),
		heartbeatFunc: client.HeartbeatFunc(),
		dispatchFunc:  client.GetDispatchMsg(),
		connConf:      connConf,
		lock:          sync.Mutex{},
		wg:            sync.WaitGroup{},
	}

	return instance
}
