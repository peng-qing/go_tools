package ltv

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"go_tools/common/network"
)

// LTVTCPConnection LTV TCP连接
type LTVTCPConnection struct {
	connID         uint64                         // 连接ID
	rwc            net.Conn                       // 原始连接
	connM          network.IConnectionManager     // 连接管理
	buffer         *bytes.Buffer                  // 缓冲区
	msgSendChan    chan []byte                    // 等待发送消息队列
	side           NetSide                        // 连接类型
	ctx            context.Context                // 上下文
	ctxCancel      context.CancelFunc             // 上下文取消
	onConnect      func(conn network.IConnection) // 连接建立回调
	onDisconnect   func(conn network.IConnection) // 连接断开回调
	protocolCoder  network.IProtocolCoder         // 协议编解码器
	heartbeatFunc  func(conn network.IConnection) // 自定义心跳函数
	dispatchFunc   func(packet network.IPacket)   // 自定义消息分发函数
	connConf       *LTVConnectionConfig           // 连接配置
	lastActiveTime time.Time                      // 最后活动时间
	lock           sync.Mutex                     // 锁
}

// Start 启动连接
func (ltv *LTVTCPConnection) Start() {
	ltv.ctx, ltv.ctxCancel = context.WithCancel(context.Background())

	// 启动主循环
	go ltv.Run()
	// 调用连接建立回调
	ltv.callOnConnect()
}

// Close 关闭连接
func (ltv *LTVTCPConnection) Close() error {
	if ltv.isClosed() {
		return errors.New("connection is closed")
	}
	if ltv.ctx != nil && ltv.ctxCancel != nil {
		ltv.ctxCancel()
	}
	// 执行回调
	if ltv.onDisconnect != nil {
		ltv.onDisconnect(ltv)
	}
	// 关闭连接
	if err := ltv.rwc.Close(); err != nil {
		slog.Error("[LTVTCPConnection] close rwc conn failed", "connID", ltv.connID, "err", err)
	}
	// 移除连接管理
	ltv.connM.RemoveByConnectionID(ltv.connID)

	return nil
}

// GetConnectionID 获取连接ID
func (ltv *LTVTCPConnection) GetConnectionID() uint64 {
	return ltv.connID
}

// RemoteAddr 获取远程地址
func (ltv *LTVTCPConnection) RemoteAddr() net.Addr {
	return ltv.rwc.RemoteAddr()
}

// LocalAddr 获取本地地址
func (ltv *LTVTCPConnection) LocalAddr() net.Addr {
	return ltv.rwc.LocalAddr()
}

// RemoteAddrString 获取远程地址字符串
func (ltv *LTVTCPConnection) RemoteAddrString() string {
	return ltv.rwc.RemoteAddr().String()
}

// LocalAddrString 获取本地地址字符串
func (ltv *LTVTCPConnection) LocalAddrString() string {
	return ltv.rwc.LocalAddr().String()
}

// IsAlive 判断连接是否存活
func (ltv *LTVTCPConnection) IsAlive() bool {
	if ltv.isClosed() {
		return false
	}
	// 最后一次活跃时间是否超过心跳间隔
	return time.Now().Sub(ltv.lastActiveTime) <= time.Duration(ltv.connConf.MaxHeartbeat)*time.Millisecond
}

// SendToQueue 发送消息
func (ltv *LTVTCPConnection) SendToQueue(data []byte) error {
	//ltv.msgSendChan <- data
	if ltv.isClosed() {
		slog.Error("[LTVTCPConnection] SendToQueue conn is closed", "connID", ltv.connID)
		return errors.New("connection is closed")
	}
	if len(data) <= 0 {
		slog.Error("[LTVTCPConnection] SendToQueue data is empty", "connID", ltv.connID)
		return errors.New("pack data is empty")
	}

	select {
	case ltv.msgSendChan <- data:
		return nil
	case <-ltv.ctx.Done():
		return errors.New("connection closed when send buff msg")
	}
}

// NewLTVServerConnection 创建LTV TCP连接 服务器
func newLTVServerConnection(connID uint64, conn net.Conn, server network.IServer, connConf *LTVConnectionConfig) *LTVTCPConnection {
	instance := &LTVTCPConnection{
		connID:        connID,
		rwc:           conn,
		side:          NodeSide_Server,
		connConf:      connConf,
		msgSendChan:   make(chan []byte, connConf.SendQueueSize),
		buffer:        bytes.NewBuffer(make([]byte, 0)),
		connM:         server.GetConnectionManager(),
		heartbeatFunc: server.HeartbeatFunc(),
		onConnect:     server.OnConnect(),
		onDisconnect:  server.OnDisconnect(),
		protocolCoder: server.ProtocolCoder(),
		dispatchFunc:  server.GetDispatchMsg(),
		lock:          sync.Mutex{},
	}

	// 注册到连接管理器
	instance.connM.Add(instance)

	return instance
}

// NewLTVClientConnection 创建LTV TCP连接 客户端
func newLTVClientConnection(client network.IClient, conn net.Conn, connConf *LTVConnectionConfig) *LTVTCPConnection {
	instance := &LTVTCPConnection{
		connID:        0, // 客户端忽略
		rwc:           conn,
		buffer:        bytes.NewBuffer(make([]byte, 0)),
		msgSendChan:   make(chan []byte, connConf.SendQueueSize),
		side:          NodeSide_Client,
		onConnect:     client.OnConnect(),
		onDisconnect:  client.OnDisconnect(),
		protocolCoder: client.ProtocolCoder(),
		heartbeatFunc: client.HeartbeatFunc(),
		dispatchFunc:  client.GetDispatchMsg(),
		connConf:      connConf,
		lock:          sync.Mutex{},
	}

	return instance
}

// isClosed 判断连接是否关闭
func (ltv *LTVTCPConnection) isClosed() bool {
	if ltv.ctx == nil || ltv.ctx.Err() != nil {
		return true
	}
	return false
}

// updateLastActivityTime 更新最后活跃时间
func (ltv *LTVTCPConnection) updateLastActiveTime() {
	ltv.lastActiveTime = time.Now()
}

// callOnConnect 调用连接建立回调
func (ltv *LTVTCPConnection) callOnConnect() {
	if ltv.onConnect != nil {
		ltv.onConnect(ltv)
	}
}

// Run 连接运行
func (ltv *LTVTCPConnection) Run() {
	// 更新活跃时间
	ltv.updateLastActiveTime()
	waitGroup := sync.WaitGroup{} // 等待组
	// 启动读写循环
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[LTVTCPConnection] Run read loop panic", "err", err, "stack", debug.Stack())
			}
		}()
		ltv.readLoop()
	}()

	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[LTVTCPConnection] Run write loop panic", "err", err, "stack", debug.Stack())
			}
		}()
		ltv.writeLoop()
	}()

	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[LTVTCPConnection] Run heartbeat loop panic", "err", err, "stack", debug.Stack())
			}
		}()
		ltv.keepalive()
	}()

	waitGroup.Wait()
}

// readLoop 读循环
func (ltv *LTVTCPConnection) readLoop() {
	// 数据缓冲区
	readBuffer := make([]byte, ltv.connConf.MaxIOReadSize)
	for {
		select {
		case <-ltv.ctx.Done():
			slog.Info("[LTVTCPConnection] read loop conn close", "connID", ltv.connID)
			return
		default:
			if ltv.isClosed() {
				slog.Info("[LTVTCPConnection] read loop conn is closed", "connID", ltv.connID)
				return
			}
			if ltv.connConf.ReadTimeout > 0 {
				_ = ltv.rwc.SetReadDeadline(time.Now().Add(time.Duration(ltv.connConf.ReadTimeout) * time.Millisecond))
			}
			// 读取网络数据
			n, err := ltv.rwc.Read(readBuffer)
			if err != nil {
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Timeout() {
					// 超时
					continue
				}
				slog.Error("[LTVTCPConnection] read message failed", "connID", ltv.connID, "readLength", n, "err", err)
				// 关闭连接
				if err := ltv.Close(); err != nil {
					slog.Error("[LTVTCPConnection] read loop read failed then close conn", "connID", ltv.connID, "err", err)
				}
				return
			}
			if ltv.connConf.ReadTimeout > 0 {
				_ = ltv.rwc.SetReadDeadline(time.Time{})
			}
			if n > 0 {
				// 成功读到对端数据 更新活跃时间
				ltv.updateLastActiveTime()
				// 写入读取数据到缓冲区
				ltv.buffer.Write(readBuffer[:n])
				// 循环解包
				for {
					packet, totalLen, decodeErr := ltv.protocolCoder.Decode(ltv.buffer.Bytes())
					if decodeErr != nil {
						// 解包失败 关闭连接
						slog.Error("[LTVTCPConnection] readLoop protocol coder decode  message failed", "connID", ltv.connID, "readLength", n, "err", err)
						if err := ltv.Close(); err != nil {
							slog.Error("[LTVTCPConnection] read loop close conn error", "connID", ltv.connID, "err", err)
						}
						return
					}
					if packet == nil && totalLen <= 0 {
						// 数据不足 等待后续包
						break
					}
					if packet != nil && totalLen > 0 {
						// 从缓冲区移除已处理数据
						ltv.buffer.Next(int(totalLen))
						// 处理完整数据包
						// packet -> dispatcher -> handler -> message -> logic
						ltv.dispatchFunc(packet)
					}
				}
			}
		}
	}
}

// writeLoop 写循环
func (ltv *LTVTCPConnection) writeLoop() {
	for {
		select {
		case <-ltv.ctx.Done():
			slog.Info("[LTVTCPConnection] write loop conn close", "connID", ltv.connID)
			close(ltv.msgSendChan)
			for data := range ltv.msgSendChan {
				ltv.rwc.Write(data)
			}
			return
		case msg := <-ltv.msgSendChan:
			if ltv.isClosed() {
				slog.Info("[LTVTCPConnection] write loop conn is closed", "connID", ltv.connID)
				return
			}
			if ltv.connConf.WriteTimeout > 0 {
				_ = ltv.rwc.SetWriteDeadline(time.Now().Add(time.Duration(ltv.connConf.WriteTimeout) * time.Millisecond))
			}
			if _, err := ltv.rwc.Write(msg); err != nil {
				slog.Error("[LTVTCPConnection] write loop write error", "connID", ltv.connID, "err", err)
				if err = ltv.Close(); err != nil {
					slog.Error("[LTVTCPConnection] write loop close conn error", "connID", ltv.connID, "err", err)
				}
				return
			}
			if ltv.connConf.WriteTimeout > 0 {
				_ = ltv.rwc.SetWriteDeadline(time.Time{})
			}
			// 写的时候是否需要更新...
			//ltv.updateLastActivityTime()
		}
	}
}

// keepalive 心跳循环
func (ltv *LTVTCPConnection) keepalive() {
	ticker := time.NewTicker(time.Duration(ltv.connConf.Heartbeat) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ltv.ctx.Done():
			slog.Info("[LTVTCPConnection] keepalive conn close", "connID", ltv.connID)
			return
		case <-ticker.C:
			if ltv.isClosed() {
				slog.Info("[LTVTCPConnection] keepalive conn is closed", "connID", ltv.connID)
				return
			}
			if !ltv.IsAlive() {
				slog.Warn("[LTVTCPConnection] keepalive not alive", "connID", ltv.connID, "lastActivity", ltv.lastActiveTime.UnixNano())
				// close conn
				if err := ltv.Close(); err != nil {
					slog.Error("[LTVTCPConnection] keepalive close conn error", "connID", ltv.connID, "err", err)
				}
				return
			}
			// 发送心跳包
			if ltv.heartbeatFunc != nil {
				ltv.heartbeatFunc(ltv)
			}
			//ltv.updateLastActivityTime()
		}
	}
}
