package ltv

import (
	"context"
	"errors"
	"github.com/gorilla/websocket"
	"go_tools/common/network"
	"log/slog"
	"net"
	"runtime/debug"
	"sync"
	"time"
)

// LTVWebsocketConnection websocket连接
type LTVWebsocketConnection struct {
	connID         uint64                         // 连接ID
	rwc            *websocket.Conn                // 原始连接
	connM          network.IConnectionManager     // 连接管理
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

func (ltv *LTVWebsocketConnection) Start() {
	ltv.ctx, ltv.ctxCancel = context.WithCancel(context.Background())
	// 启动主循环
	go ltv.Run()

	ltv.callOnConnect()
}

func (ltv *LTVWebsocketConnection) Close() error {
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
	if err := ltv.rwc.Close(); err != nil {
		slog.Error("[LTVTCPConnection] close rwc conn failed", "connID", ltv.connID, "err", err)
	}
	// 移除连接
	ltv.connM.RemoveByConnectionID(ltv.connID)

	return nil
}

func (ltv *LTVWebsocketConnection) GetConnectionID() uint64 {
	return ltv.connID
}

func (ltv *LTVWebsocketConnection) RemoteAddr() net.Addr {
	return ltv.rwc.RemoteAddr()
}

func (ltv *LTVWebsocketConnection) LocalAddr() net.Addr {
	return ltv.rwc.LocalAddr()
}

func (ltv *LTVWebsocketConnection) RemoteAddrString() string {
	return ltv.rwc.RemoteAddr().String()
}

func (ltv *LTVWebsocketConnection) LocalAddrString() string {
	return ltv.rwc.LocalAddr().String()
}

func (ltv *LTVWebsocketConnection) IsAlive() bool {
	if ltv.isClosed() {
		return false
	}
	return time.Now().Sub(ltv.lastActiveTime) <= time.Duration(ltv.connConf.MaxHeartbeat)*time.Millisecond
}

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

// callOnConnect 连接建立回调
func (ltv *LTVWebsocketConnection) callOnConnect() {
	if ltv.onConnect != nil {
		ltv.onConnect(ltv)
	}
}

// Run 连接主循环
func (ltv *LTVWebsocketConnection) Run() {
	// 更新活跃时间
	ltv.updateLastActiveTime()
	// 启动读写循环
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[LTVWebsocketConnection] Run read loop panic", "err", err, "stack", debug.Stack())
			}
		}()
		ltv.readLoop()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[LTVWebsocketConnection] Run write loop panic", "err", err, "stack", debug.Stack())
			}
		}()
		ltv.writeLoop()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[LTVWebsocketConnection] Run heartbeat loop panic", "err", err, "stack", debug.Stack())
			}
		}()
		ltv.keepalive()
	}()

	wg.Wait()
}

func (ltv *LTVWebsocketConnection) updateLastActiveTime() {
	ltv.lastActiveTime = time.Now()
}

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
					ltv.dispatchFunc(packet)
				}
			}
		}
	}
}

func (ltv *LTVWebsocketConnection) writeLoop() {
	for {
		select {
		case <-ltv.ctx.Done():
			slog.Info("[LTVWebsocketConnection] write loop conn close", "connID", ltv.connID)
			close(ltv.msgSendChan)
			for data := range ltv.msgSendChan {
				if err := ltv.rwc.WriteMessage(websocket.BinaryMessage, data); err != nil {
					slog.Error("[LTVWebsocketConnection] write loop write error", "connID", ltv.connID, "err", err, "message")
					return
				}
			}
			return
		case msg := <-ltv.msgSendChan:
			if ltv.isClosed() {
				slog.Info("[LTVWebsocketConnection] write loop conn is closed", "connID", ltv.connID)
				return
			}
			if ltv.connConf.WriteTimeout > 0 {
				_ = ltv.rwc.SetWriteDeadline(time.Now().Add(time.Duration(ltv.connConf.WriteTimeout) * time.Millisecond))
			}
			// TODO add lock
			if err := ltv.rwc.WriteMessage(websocket.BinaryMessage, msg); err != nil {
				slog.Error("[LTVWebsocketConnection] write loop write error", "connID", ltv.connID, "err", err)
				if err = ltv.Close(); err != nil {
					slog.Error("[LTVWebsocketConnection] write loop close conn error", "connID", ltv.connID, "err", err)
				}
				return
			}
			if ltv.connConf.WriteTimeout > 0 {
				_ = ltv.rwc.SetWriteDeadline(time.Time{})
			}
		}
	}
}

// isClosed 判断连接是否关闭
func (ltv *LTVWebsocketConnection) isClosed() bool {
	if ltv.ctx == nil || ltv.ctx.Err() != nil {
		return true
	}
	return false
}

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
