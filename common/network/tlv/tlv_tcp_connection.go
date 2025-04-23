package tlv

import (
	"context"
	"log/slog"
	"net"
	"runtime/debug"
	"sync"
	"time"

	"go_tools/common/network"
	"go_tools/common/options"
)

// TLVTCPConnection TLV TCP连接
type TLVTCPConnection struct {
	connID           uint64                         // 连接ID
	rwc              net.Conn                       // 原始连接
	msgSendChan      chan []byte                    // 等待发送消息队列
	side             network.NetSide                // 连接类型
	ctx              context.Context                // 上下文
	ctxCancel        context.CancelFunc             // 上下文取消
	onConnect        func(conn network.IConnection) // 连接建立回调
	onDisconnect     func(conn network.IConnection) // 连接断开回调
	protocolCoder    network.IProtocolCoder         // 协议编解码器
	heartbeat        time.Duration                  // 心跳间隔
	heartbeatFunc    func(conn network.IConnection) // 自定义心跳函数
	readTimeout      time.Duration                  // 读超时时间
	writeTimeout     time.Duration                  // 写超时时间
	lastActivityTime time.Time                      // 最后活动时间
	lock             sync.Mutex                     // 锁
}

// Start 启动连接
func (tlv *TLVTCPConnection) Start() {
	// 调用连接建立回调
	tlv.callOnConnect()
	// 启动主循环
	tlv.Run()
}

// Close 关闭连接
func (tlv *TLVTCPConnection) Close() error {
	if tlv.ctx != nil && tlv.ctxCancel != nil {
		tlv.ctxCancel()
	}

	return nil
}

// GetConnectionID 获取连接ID
func (tlv *TLVTCPConnection) GetConnectionID() uint64 {
	return tlv.connID
}

// RemoteAddr 获取远程地址
func (tlv *TLVTCPConnection) RemoteAddr() net.Addr {
	return tlv.rwc.RemoteAddr()
}

// LocalAddr 获取本地地址
func (tlv *TLVTCPConnection) LocalAddr() net.Addr {
	return tlv.rwc.LocalAddr()
}

// RemoteAddrString 获取远程地址字符串
func (tlv *TLVTCPConnection) RemoteAddrString() string {
	return tlv.rwc.RemoteAddr().String()
}

// LocalAddrString 获取本地地址字符串
func (tlv *TLVTCPConnection) LocalAddrString() string {
	return tlv.rwc.LocalAddr().String()
}

// IsAlive 判断连接是否存活
func (tlv *TLVTCPConnection) IsAlive() bool {
	if tlv.isClosed() {
		return false
	}
	// 最后一次活跃时间是否超过心跳间隔
	return time.Now().Sub(tlv.lastActivityTime) < tlv.heartbeat
}

// NewTLVServerConnection 创建TLV TCP连接
func newTLVServerConnection(server network.IServer, conn net.Conn, connID uint64, heartbeat time.Duration, opts ...options.Option[TLVTCPConnection]) *TLVTCPConnection {
	ctx, ctxCancel := context.WithCancel(context.Background())

	instance := &TLVTCPConnection{
		connID:        connID,
		rwc:           conn,
		side:          network.NodeSide_Server,
		heartbeat:     heartbeat,
		ctx:           ctx,
		ctxCancel:     ctxCancel,
		heartbeatFunc: server.HeartbeatFunc(),
		onConnect:     server.OnConnect(),
		onDisconnect:  server.OnDisconnect(),
		protocolCoder: server.ProtocolCoder(),
	}

	for _, option := range opts {
		option.Apply(instance)
	}

	return instance
}

// isClosed 判断连接是否关闭
func (tlv *TLVTCPConnection) isClosed() bool {
	if tlv.ctx == nil || tlv.ctx.Err() != nil {
		return true
	}
	return false
}

// updateLastActivityTime 更新最后活跃时间
func (tlv *TLVTCPConnection) updateLastActivityTime() {
	tlv.lastActivityTime = time.Now()
}

// callOnConnect 调用连接建立回调
func (tlv *TLVTCPConnection) callOnConnect() {
	if tlv.onConnect != nil {
		tlv.onConnect(tlv)
	}
}

// Run 连接运行
func (tlv *TLVTCPConnection) Run() {
	waitGroup := sync.WaitGroup{} // 等待组
	// 启动读写循环
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[TLVTCPConnection] Run read loop panic", "err", err, "stack", debug.Stack())
			}
		}()
		tlv.readLoop()
	}()

	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[TLVTCPConnection] Run write loop panic", "err", err, "stack", debug.Stack())
			}
		}()
		tlv.writeLoop()
	}()

	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[TLVTCPConnection] Run heartbeat loop panic", "err", err, "stack", debug.Stack())
			}
		}()
		tlv.keepalive()
	}()

	waitGroup.Wait()
}

// readLoop 读循环
func (tlv *TLVTCPConnection) readLoop() {
	//	TODO 实现 read loop
}

// writeLoop 写循环
func (tlv *TLVTCPConnection) writeLoop() {
	for {
		select {
		case <-tlv.ctx.Done():
			slog.Info("[TLVTCPConnection] write loop conn close", "connID", tlv.connID)
			close(tlv.msgSendChan)
			return
		case msg := <-tlv.msgSendChan:
			if tlv.isClosed() {
				slog.Info("[TLVTCPConnection] write loop conn is closed", "connID", tlv.connID)
				return
			}
			if tlv.writeTimeout > 0 {
				_ = tlv.rwc.SetWriteDeadline(time.Now().Add(tlv.writeTimeout))
			}
			if _, err := tlv.rwc.Write(msg); err != nil {
				slog.Error("[TLVTCPConnection] write loop write error", "connID", tlv.connID, "err", err)
			}
			if tlv.writeTimeout > 0 {
				_ = tlv.rwc.SetWriteDeadline(time.Time{})
			}
			// 写的时候是否需要更新...
			//tlv.updateLastActivityTime()
		}
	}
}

// keepalive 心跳循环
func (tlv *TLVTCPConnection) keepalive() {
	ticker := time.NewTicker(tlv.heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-tlv.ctx.Done():
			slog.Info("[TLVTCPConnection] keepalive conn close", "connID", tlv.connID)
			return
		case <-ticker.C:
			if tlv.isClosed() {
				slog.Info("[TLVTCPConnection] keepalive conn is closed", "connID", tlv.connID)
				return
			}
			if !tlv.IsAlive() {
				slog.Warn("[TLVTCPConnection] keepalive not alive", "connID", tlv.connID)
				// close conn
				if err := tlv.Close(); err != nil {
					slog.Error("[TLVTCPConnection] keepalive close conn error", "connID", tlv.connID, "err", err)
				}
				return
			}
			// 发送心跳包
			if tlv.heartbeatFunc != nil {
				tlv.heartbeatFunc(tlv)
			}
			tlv.updateLastActivityTime()
		}
	}
}
