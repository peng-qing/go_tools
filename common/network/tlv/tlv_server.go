package tlv

import (
	"context"
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

type TLVServer struct {
	serverID      uint64                         // 服务器ID
	cIDGenerator  uint64                         // 连接ID生成器
	srvConf       *TLVServerConfig               // 服务器配置
	connM         network.IConnectionManager     // 连接管理器
	protocolCoder network.IProtocolCoder         // 协议编码器
	ctx           context.Context                // 上下文
	ctxCancel     context.CancelFunc             // 上下文取消函数
	onConnect     func(conn network.IConnection) // 连接回调
	onDisconnect  func(conn network.IConnection) // 断开连接回调
	heartbeatFunc func(conn network.IConnection) // 心跳函数
	timerM        *timer.TimerManager            // 定时器管理器
	ticker        *time.Ticker                   // 定时器
	upgrader      *websocket.Upgrader            // websocket升级器
	waitGroup     sync.WaitGroup                 // WaitGroup
}

// NewTLVServer 创建TLV服务器
func NewTLVServer(srvConf *TLVServerConfig, opts ...options.Option[TLVServer]) *TLVServer {
	ctx, ctxCancel := context.WithCancel(context.Background())
	protocolCoder := NewTLVProtocolCoder(srvConf.UsedLittleEndian)
	connM := NewTLVConnectionManager()
	instance := &TLVServer{
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
func (tlv *TLVServer) Serve() {
	slog.Info("[TLVServer] Serve", "ServerID", tlv.serverID, "IP", tlv.srvConf.IP, "Port", tlv.srvConf.Port, "WsPort", tlv.srvConf.WsPort)
	// 启动主循环
	tlv.Run()
}

func (tlv *TLVServer) Close() error {
	if tlv.ctx != nil && tlv.ctxCancel != nil {
		tlv.ctxCancel()
	}

	// 等待所有子协程退出
	tlv.waitGroup.Wait()
	return nil
}

// SetOnConnect 设置连接回调
func (tlv *TLVServer) SetOnConnect(fn func(conn network.IConnection)) {
	tlv.onConnect = fn
}

// OnConnect 获取连接回调
func (tlv *TLVServer) OnConnect() func(conn network.IConnection) {
	return tlv.onConnect
}

// SetOnDisconnect 设置断开连接回调
func (tlv *TLVServer) SetOnDisconnect(fn func(conn network.IConnection)) {
	tlv.onDisconnect = fn
}

// OnDisconnect 获取断开连接回调
func (tlv *TLVServer) OnDisconnect() func(conn network.IConnection) {
	return tlv.onDisconnect
}

// ProtocolCoder 获取协议编码器
func (tlv *TLVServer) ProtocolCoder() network.IProtocolCoder {
	return tlv.protocolCoder
}

func (tlv *TLVServer) HeartbeatFunc() func(conn network.IConnection) {
	return tlv.heartbeatFunc
}

func (tlv *TLVServer) SetHeartbeatFunc(fn func(conn network.IConnection)) {
	tlv.heartbeatFunc = fn
}

func (tlv *TLVServer) GetDispatchMsg() func(packet network.IPacket) {
	return nil
}

// Run 服务器主循环
func (tlv *TLVServer) Run() {
	slog.Info("[TLVServer] Run", "serverID", tlv.serverID, "conf", tlv.srvConf)
	tlv.waitGroup.Add(1)
	go func() {
		defer tlv.waitGroup.Done()
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[TLVServer] Run panic", "err", err, "stack", debug.Stack())
			}
		}()
		// 服务主循环
		tlv.Loop()
	}()

	// 监听连接
	tlv.ListenConn()
}

func (tlv *TLVServer) Loop() {
	for {
		select {
		case <-tlv.ctx.Done():
			// 退出
			slog.Info("[TLVServer] Loop exit....", "ServerID", tlv.serverID)
			return
		case now := <-tlv.ticker.C:
			// 执行定时器
			tlv.timerM.Run(now.Unix(), 0)
		}
	}
}

// ListenConn 监听连接
func (tlv *TLVServer) ListenConn() {
	slog.Info("[TLVServer] ListenConn", "ServerID", tlv.serverID, "Mode", tlv.srvConf.Mode,
		"IP", tlv.srvConf.IP, "Port", tlv.srvConf.Port, "WsPort", tlv.srvConf.WsPort)
	switch tlv.srvConf.Mode {
	case network.Tcp:
		go tlv.ListenTCPConn()
	case network.Websocket:
		go tlv.ListenWebsocketConn()
	default:
		go tlv.ListenTCPConn()
		go tlv.ListenWebsocketConn()
	}
}

// ListenWebsocketConn 监听Websocket连接
func (tlv *TLVServer) ListenWebsocketConn() {
	tlv.waitGroup.Add(1)
	defer tlv.waitGroup.Done()

	slog.Info("[TLVServer] ListenWebsocketConn", "ServerID", tlv.serverID, "IP", tlv.srvConf.IP,
		"WsPort", tlv.srvConf.WsPort, "WsPath", tlv.srvConf.WsPath)
	http.HandleFunc(tlv.srvConf.WsPath, func(w http.ResponseWriter, r *http.Request) {
		// 检查超过最大连接
		if tlv.connM.Count() >= tlv.srvConf.MaxConn {
			slog.Info("[TLVServer] session count out of limit")
			network.AcceptDelay.Delay()
			return
		}

		// 升级成 websocket 连接
		conn, err := tlv.upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Error("[TLVServer] ListenWebsocketConn upgrade websocket error", "ServerID", tlv.serverID, "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			network.AcceptDelay.Delay()
			return
		}
		network.AcceptDelay.Reset()
		//TODO 创建 IConnection
		cID := atomic.AddUint64(&tlv.cIDGenerator, 1)
		conn.WriteJSON(cID)
	})
}

func (tlv *TLVServer) ListenTCPConn() {
	tlv.waitGroup.Add(1)
	defer tlv.waitGroup.Done()

	slog.Info("[TLVServer] ListenTCPConn", "ServerID", tlv.serverID, "IP", tlv.srvConf.IP, "Port", tlv.srvConf.Port)
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", tlv.srvConf.IP, tlv.srvConf.Port))
	if err != nil {
		slog.Error("[TLVServer] ListenTCPConn resolve tcp addr error", "ServerID", tlv.serverID, "err", err)
		panic(err)
	}
	tcpListener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		slog.Error("[TLVServer] ListenTCPConn listen tcp error", "ServerID", tlv.serverID, "err", err)
		panic(err)
	}
	for {
		select {
		case <-tlv.ctx.Done():
			// 退出
			if err = tcpListener.Close(); err != nil {
				slog.Error("[TLVServer] ListenTCPConn close listener error", "ServerID", tlv.serverID, "err", err)
			}
			return
		default:
			if tlv.connM.Count() >= tlv.srvConf.MaxConn {
				slog.Info("[TLVServer] session count out of limit", "ServerID", tlv.serverID, "CurCount", tlv.connM.Count())
				network.AcceptDelay.Delay()
				continue
			}
			tcpConn, err := tcpListener.Accept()
			if err != nil {
				slog.Error("[TLVServer] ListenTCPConn accept tcp error", "ServerID", tlv.serverID, "err", err)
				continue
			}
			network.AcceptDelay.Reset()
			cID := atomic.AddUint64(&tlv.cIDGenerator, 1)
			dealConn := newTLVServerConnection(cID, tcpConn, tlv, tlv.srvConf.Connection)
			// TODO 注册到连接管理器
			dealConn.Start()
		}
	}
}
