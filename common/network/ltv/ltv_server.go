package ltv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/peng-qing/go_tools/common/network"
	"github.com/peng-qing/go_tools/common/options"
	"github.com/peng-qing/go_tools/common/timer"

	"github.com/gorilla/websocket"
)

var (
	// 断言 检查是否实现 network.IServer
	_ network.IServer = (*LTVServer)(nil)
)

// LTVServer LTV服务器
type LTVServer struct {
	serverID      uint64                                                 // 服务器ID
	cIDGenerator  uint64                                                 // 连接ID生成器
	srvConf       *LTVServerConfig                                       // 服务器配置
	connM         network.IConnectionManager                             // 连接管理器
	protocolCoder network.IProtocolCoder                                 // 协议编码器
	ctx           context.Context                                        // 上下文
	ctxCancel     context.CancelFunc                                     // 上下文取消函数
	onConnect     func(conn network.IConnection)                         // 连接回调
	onDisconnect  func(conn network.IConnection)                         // 断开连接回调
	heartbeatFunc func(conn network.IConnection)                         // 心跳函数
	dispatchFunc  func(conn network.IConnection, packet network.IPacket) // 消息处理函数
	timerM        *timer.TimerManager                                    // 定时器管理器
	ticker        *time.Ticker                                           // 定时器
	upgrader      *websocket.Upgrader                                    // websocket升级器
	waitGroup     sync.WaitGroup                                         // WaitGroup
}

// ===============================================================================
// 实现 network.IServer 接口
// ===============================================================================

// Serve 启动服务器
func (ltv *LTVServer) Serve() {
	slog.Info("[LTVServer] Serve", "ServerID", ltv.serverID, "IP", ltv.srvConf.IP, "Port", ltv.srvConf.Port, "WsPort", ltv.srvConf.WsPort)
	// 启动主循环
	ltv.run()
}

// Close 关闭服务器
func (ltv *LTVServer) Close() error {
	if ltv.isClosed() {
		return errors.New("server repeat close")
	}
	if ltv.ctx != nil && ltv.ctxCancel != nil {
		ltv.ctxCancel()
	}

	// 关闭所有连接
	err := ltv.connM.Range(func(connID uint64, conn network.IConnection) error {
		if err := conn.Close(); err != nil {
			slog.Error("[LTVServer] Close conn failed", "connID", connID, "err", err)
		}
		return nil
	})

	// 等待所有子协程退出
	ltv.waitGroup.Wait()
	return err
}

// GetConnectionManager 获取连接管理器
func (ltv *LTVServer) GetConnectionManager() network.IConnectionManager {
	return ltv.connM
}

// SetOnConnect 设置连接回调
func (ltv *LTVServer) SetOnConnect(fn func(conn network.IConnection)) {
	ltv.onConnect = fn
}

// OnConnect 获取连接回调
func (ltv *LTVServer) OnConnect() func(conn network.IConnection) {
	return ltv.onConnect
}

// SetOnDisconnect 设置断开连接回调
func (ltv *LTVServer) SetOnDisconnect(fn func(conn network.IConnection)) {
	ltv.onDisconnect = fn
}

// OnDisconnect 获取断开连接回调
func (ltv *LTVServer) OnDisconnect() func(conn network.IConnection) {
	return ltv.onDisconnect
}

// ProtocolCoder 获取协议编码器
func (ltv *LTVServer) ProtocolCoder() network.IProtocolCoder {
	return ltv.protocolCoder
}

// HeartbeatFunc 获取心跳函数
func (ltv *LTVServer) HeartbeatFunc() func(conn network.IConnection) {
	return ltv.heartbeatFunc
}

// SetHeartbeatFunc 设置心跳函数
func (ltv *LTVServer) SetHeartbeatFunc(fn func(conn network.IConnection)) {
	ltv.heartbeatFunc = fn
}

// SetDispatchMsg 设置消息处理函数
func (ltv *LTVServer) SetDispatchMsg(fn func(conn network.IConnection, packet network.IPacket)) {
	ltv.dispatchFunc = fn
}

// GetDispatchMsg 获取消息处理函数
func (ltv *LTVServer) GetDispatchMsg() func(conn network.IConnection, packet network.IPacket) {
	return ltv.dispatchFunc
}

// AddTimer 添加定时器
func (ltv *LTVServer) AddTimer(callback timer.TimeOuter, endTm int64, interval int64) int64 {
	if callback == nil {
		slog.Error("[LTVServer] AddTimer register nil callback")
		return 0
	}
	slog.Info("[LTVServer] AddTimer register timer success", "endTm", endTm, "interval", interval)
	return ltv.timerM.AddTimer(callback, endTm, interval)
}

// ===============================================================================
// 私有接口
// ===============================================================================

// Run 服务器主循环
func (ltv *LTVServer) run() {
	slog.Info("[LTVServer] Run", "serverID", ltv.serverID, "conf", ltv.srvConf)
	ltv.waitGroup.Add(1)
	go func() {
		defer ltv.waitGroup.Done()
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[LTVServer] Run panic", "err", err, "stack", debug.Stack())
			}
		}()
		// 定时器主循环
		ltv.timerLoop()
	}()

	// 监听连接
	ltv.listenConn()
}

// timerLoop 定时器主循环
func (ltv *LTVServer) timerLoop() {
	for {
		select {
		case <-ltv.ctx.Done():
			// 退出
			slog.Info("[LTVServer] timerLoop exit....", "ServerID", ltv.serverID)
			return
		case now := <-ltv.ticker.C:
			// 执行定时器
			ltv.timerM.Run(now.Unix(), 0)
		}
	}
}

// listenConn 监听连接
func (ltv *LTVServer) listenConn() {
	slog.Info("[LTVServer] listenConn", "ServerID", ltv.serverID, "Mode", ltv.srvConf.Mode,
		"IP", ltv.srvConf.IP, "Port", ltv.srvConf.Port, "WsPort", ltv.srvConf.WsPort)
	switch ltv.srvConf.Mode {
	case NetMode_Tcp:
		go ltv.listenTCPConn()
	case NetMode_Websocket:
		go ltv.listenWebsocketConn()
	default:
		go ltv.listenTCPConn()
		go ltv.listenWebsocketConn()
	}
}

// listenWebsocketConn 监听Websocket连接
func (ltv *LTVServer) listenWebsocketConn() {
	slog.Info("[LTVServer] listenWebsocketConn", "ServerID", ltv.serverID, "IP", ltv.srvConf.IP,
		"WsPort", ltv.srvConf.WsPort, "WsPath", ltv.srvConf.WsPath)
	http.HandleFunc(ltv.srvConf.WsPath, func(w http.ResponseWriter, r *http.Request) {
		// 检查超过最大连接
		if ltv.connM.Count() >= ltv.srvConf.MaxConn {
			slog.Info("[LTVServer] session count out of limit")
			network.AcceptDelay.Delay()
			return
		}

		// 升级成 websocket 连接
		conn, err := ltv.upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("[LTVServer] ListenWebsocketConn upgrade websocket error", "ServerID", ltv.serverID, "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			network.AcceptDelay.Delay()
			return
		}
		network.AcceptDelay.Reset()
		cID := atomic.AddUint64(&ltv.cIDGenerator, 1)
		dealConn := newLTVServerWebsocketConnection(cID, conn, ltv, ltv.srvConf.Connection)
		dealConn.Start()
	})

	err := http.ListenAndServe(fmt.Sprintf("%s:%d", ltv.srvConf.IP, ltv.srvConf.WsPort), nil)
	if err != nil {
		slog.Error("[LTVServer] ListenWebsocketConn listen websocket error", "ServerID", ltv.serverID, "err", err)
		panic(err)
	}
}

// listenTCPConn 监听TCP连接
func (ltv *LTVServer) listenTCPConn() {
	slog.Info("[LTVServer] listenTCPConn", "ServerID", ltv.serverID, "IP", ltv.srvConf.IP, "Port", ltv.srvConf.Port)
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", ltv.srvConf.IP, ltv.srvConf.Port))
	if err != nil {
		slog.Error("[LTVServer] listenTCPConn resolve tcp addr error", "ServerID", ltv.serverID, "err", err)
		panic(err)
	}
	tcpListener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		slog.Error("[LTVServer] listenTCPConn listen tcp error", "ServerID", ltv.serverID, "err", err)
		panic(err)
	}

	// AcceptTCP 会阻塞协程 单开一个协程处理
	go func() {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[LTVServer] listenTCPConn panic", "err", err, "stack", debug.Stack())
			}
		}()
		for {
			if ltv.connM.Count() >= ltv.srvConf.MaxConn {
				slog.Info("[LTVServer] listenTCPConn session count out of limit", "ServerID", ltv.serverID, "CurCount", ltv.connM.Count())
				network.AcceptDelay.Delay()
				continue
			}
			tcpConn, err := tcpListener.AcceptTCP()
			if err != nil {
				// 关闭
				if errors.Is(err, net.ErrClosed) {
					slog.Info("[LTVServer] listenTCPConn listener closed", "ServerID", ltv.serverID)
					return
				}
				slog.Error("[LTVServer] listenTCPConn accept tcp error", "ServerID", ltv.serverID, "err", err)
				network.AcceptDelay.Delay()
				continue
			}
			if err = tcpConn.SetNoDelay(true); err != nil {
				slog.Error("[LTVServer] listenTCPConn set no delay error", "ServerID", ltv.serverID, "err", err)
				return
			}
			network.AcceptDelay.Reset()
			cID := atomic.AddUint64(&ltv.cIDGenerator, 1)
			dealConn := newLTVServerConnection(cID, tcpConn, ltv, ltv.srvConf.Connection)
			dealConn.Start()
		}
	}()

	// 监听关闭信号
	select {
	case <-ltv.ctx.Done():
		// 退出
		if err = tcpListener.Close(); err != nil {
			slog.Error("[LTVServer] listenTCPConn close listener error", "ServerID", ltv.serverID, "err", err)
		}
		return
	}
}

// isClosed 判断服务器是否关闭
func (ltv *LTVServer) isClosed() bool {
	if ltv.ctx == nil || ltv.ctx.Err() != nil {
		return true
	}
	return false
}

// ===============================================================================
// 实例化接口
// ===============================================================================

// NewLTVServer 创建LTV服务器
func NewLTVServer(serverID uint64, srvConf *LTVServerConfig, opts ...options.Option[LTVServer]) *LTVServer {
	ctx, ctxCancel := context.WithCancel(context.Background())
	protocolCoder := NewLTVProtocolCoder(srvConf.UsedLittleEndian)
	connM := NewLTVConnectionManager()
	instance := &LTVServer{
		serverID:      serverID,
		cIDGenerator:  0,
		srvConf:       srvConf,
		protocolCoder: protocolCoder,
		connM:         connM,
		ctx:           ctx,
		ctxCancel:     ctxCancel,
		timerM:        timer.NewTimerManager(srvConf.TimerQueueSize),
		ticker:        time.NewTicker(time.Duration(srvConf.Frequency) * time.Millisecond),
		upgrader: &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
			ReadBufferSize: srvConf.Connection.MaxIOReadSize,
		},
		waitGroup: sync.WaitGroup{},
	}

	for _, opt := range opts {
		opt.Apply(instance)
	}

	return instance
}
