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

	"go_tools/common/network"
	"go_tools/common/options"
	"go_tools/common/timer"

	"github.com/gorilla/websocket"
)

type LTVServer struct {
	serverID      uint64                         // 服务器ID
	cIDGenerator  uint64                         // 连接ID生成器
	srvConf       *LTVServerConfig               // 服务器配置
	connM         network.IConnectionManager     // 连接管理器
	protocolCoder network.IProtocolCoder         // 协议编码器
	ctx           context.Context                // 上下文
	ctxCancel     context.CancelFunc             // 上下文取消函数
	onConnect     func(conn network.IConnection) // 连接回调
	onDisconnect  func(conn network.IConnection) // 断开连接回调
	heartbeatFunc func(conn network.IConnection) // 心跳函数
	dispatchFunc  func(packet network.IPacket)   // 消息处理函数
	timerM        *timer.TimerManager            // 定时器管理器
	ticker        *time.Ticker                   // 定时器
	upgrader      *websocket.Upgrader            // websocket升级器
	waitGroup     sync.WaitGroup                 // WaitGroup
}

// NewLTVServer 创建LTV服务器
func NewLTVServer(srvConf *LTVServerConfig, opts ...options.Option[LTVServer]) *LTVServer {
	ctx, ctxCancel := context.WithCancel(context.Background())
	protocolCoder := NewLTVProtocolCoder(srvConf.UsedLittleEndian)
	connM := NewLTVConnectionManager()
	instance := &LTVServer{
		serverID:      srvConf.ServerID,
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

// Serve 启动服务器
func (ltv *LTVServer) Serve() {
	slog.Info("[LTVServer] Serve", "ServerID", ltv.serverID, "IP", ltv.srvConf.IP, "Port", ltv.srvConf.Port, "WsPort", ltv.srvConf.WsPort)
	// 启动主循环
	ltv.Run()
}

func (ltv *LTVServer) Close() error {
	if ltv.ctx != nil && ltv.ctxCancel != nil {
		ltv.ctxCancel()
	}

	// 等待所有子协程退出
	ltv.waitGroup.Wait()
	return nil
}

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
func (ltv *LTVServer) SetDispatchMsg(fn func(packet network.IPacket)) {
	ltv.dispatchFunc = fn
}

func (ltv *LTVServer) GetDispatchMsg() func(packet network.IPacket) {
	return nil
}

// Run 服务器主循环
func (ltv *LTVServer) Run() {
	slog.Info("[LTVServer] Run", "serverID", ltv.serverID, "conf", ltv.srvConf)
	ltv.waitGroup.Add(1)
	go func() {
		defer ltv.waitGroup.Done()
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[LTVServer] Run panic", "err", err, "stack", debug.Stack())
			}
		}()
		// 服务主循环
		ltv.Loop()
	}()

	// 监听连接
	ltv.ListenConn()
}

func (ltv *LTVServer) Loop() {
	for {
		select {
		case <-ltv.ctx.Done():
			// 退出
			slog.Info("[LTVServer] Loop exit....", "ServerID", ltv.serverID)
			return
		case now := <-ltv.ticker.C:
			// 执行定时器
			ltv.timerM.Run(now.Unix(), 0)
		}
	}
}

// ListenConn 监听连接
func (ltv *LTVServer) ListenConn() {
	slog.Info("[LTVServer] ListenConn", "ServerID", ltv.serverID, "Mode", ltv.srvConf.Mode,
		"IP", ltv.srvConf.IP, "Port", ltv.srvConf.Port, "WsPort", ltv.srvConf.WsPort)
	switch ltv.srvConf.Mode {
	case Tcp:
		go ltv.ListenTCPConn()
	case Websocket:
		go ltv.ListenWebsocketConn()
	default:
		go ltv.ListenTCPConn()
		go ltv.ListenWebsocketConn()
	}
}

// ListenWebsocketConn 监听Websocket连接
func (ltv *LTVServer) ListenWebsocketConn() {
	ltv.waitGroup.Add(1)
	defer ltv.waitGroup.Done()

	slog.Info("[LTVServer] ListenWebsocketConn", "ServerID", ltv.serverID, "IP", ltv.srvConf.IP,
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
		//TODO 创建 IConnection
		cID := atomic.AddUint64(&ltv.cIDGenerator, 1)
		conn.WriteJSON(cID)
	})
}

func (ltv *LTVServer) ListenTCPConn() {
	ltv.waitGroup.Add(1)
	defer ltv.waitGroup.Done()

	slog.Info("[LTVServer] ListenTCPConn", "ServerID", ltv.serverID, "IP", ltv.srvConf.IP, "Port", ltv.srvConf.Port)
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", ltv.srvConf.IP, ltv.srvConf.Port))
	if err != nil {
		slog.Error("[LTVServer] ListenTCPConn resolve tcp addr error", "ServerID", ltv.serverID, "err", err)
		panic(err)
	}
	tcpListener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		slog.Error("[LTVServer] ListenTCPConn listen tcp error", "ServerID", ltv.serverID, "err", err)
		panic(err)
	}
	for {
		select {
		case <-ltv.ctx.Done():
			// 退出
			if err = tcpListener.Close(); err != nil {
				slog.Error("[LTVServer] ListenTCPConn close listener error", "ServerID", ltv.serverID, "err", err)
			}
			return
		default:
			if ltv.connM.Count() >= ltv.srvConf.MaxConn {
				slog.Info("[LTVServer] session count out of limit", "ServerID", ltv.serverID, "CurCount", ltv.connM.Count())
				network.AcceptDelay.Delay()
				continue
			}
			tcpConn, err := tcpListener.AcceptTCP()
			if err != nil {
				var netErr net.Error
				if errors.As(err, &netErr) && netErr.Temporary() {
					continue
				}
				slog.Error("[LTVServer] ListenTCPConn accept tcp error", "ServerID", ltv.serverID, "err", err)
				continue
			}
			if err := tcpConn.SetNoDelay(true); err != nil {
				slog.Error("[LTVServer] ListenTCPConn set no delay error", "ServerID", ltv.serverID, "err", err)
				return
			}
			network.AcceptDelay.Reset()
			cID := atomic.AddUint64(&ltv.cIDGenerator, 1)
			dealConn := newLTVServerConnection(cID, tcpConn, ltv, ltv.srvConf.Connection)
			dealConn.Start()
		}
	}
}
