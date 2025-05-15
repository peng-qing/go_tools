package ltv

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"runtime/debug"
	"sync"

	"go_tools/common/network"
	"go_tools/common/options"

	"github.com/gorilla/websocket"
)

var (
	// 断言 检查是否实现 network.IClient 接口
	_ network.IClient = (*LTVClient)(nil)
)

// LTVClient ltv客户端
type LTVClient struct {
	URL           *url.URL                                               //  url
	ErrChan       chan error                                             // 错误通道
	cliConf       *LTVClientConfig                                       // 客户端配置
	conn          network.IConnection                                    // 连接
	protocolCoder network.IProtocolCoder                                 // 协议编码器
	onConnect     func(conn network.IConnection)                         // 连接成功回调
	onDisconnect  func(conn network.IConnection)                         // 连接断开回调
	heartbeatFunc func(conn network.IConnection)                         // 心跳函数
	dispatchFunc  func(conn network.IConnection, packet network.IPacket) // 消息分发函数
	wsDialer      *websocket.Dialer                                      // websocket拨号器
	ctx           context.Context                                        // 上下文
	ctxCancel     context.CancelFunc                                     // 上下文取消函数
	waitGroup     sync.WaitGroup                                         // 等待组
	lock          sync.Mutex                                             // 锁
}

// ===============================================================================
// 实现 network.IClient 接口
// ===============================================================================

// Start 启动客户端
func (ltv *LTVClient) Start() {
	slog.Info("[LTVClient] Start...", "IP", ltv.cliConf.IP, "Port", ltv.cliConf.Port, "Mode", ltv.cliConf.Mode)
	ltv.Restart()
}

// Close 关闭客户端
func (ltv *LTVClient) Close() error {
	var err error
	// 关闭连接
	if conn := ltv.Connection(); conn != nil {
		err = conn.Close()
	}
	// 通知子协程退出
	if ltv.ctx != nil && ltv.ctx.Err() == nil {
		ltv.ctxCancel()
	}
	// 等待子协程退出完成
	ltv.waitGroup.Wait()
	close(ltv.ErrChan)

	slog.Info("[LTVClient] Close success...", "IP", ltv.cliConf.IP, "Port", ltv.cliConf.Port, "Mode", ltv.cliConf.Mode)
	return err
}

// Restart 重启客户端
func (ltv *LTVClient) Restart() {
	ltv.ctx, ltv.ctxCancel = context.WithCancel(context.Background())
	ltv.waitGroup.Add(1)
	go func() {
		defer ltv.waitGroup.Done()
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[LTVClient] Run main loop panic", "err", err, "stack", debug.Stack())
			}
		}()
		ltv.run()
	}()
}

// Connection 获取连接
func (ltv *LTVClient) Connection() network.IConnection {
	ltv.lock.Lock()
	defer ltv.lock.Unlock()

	return ltv.conn
}

// SetConnection 设置连接
func (ltv *LTVClient) SetConnection(conn network.IConnection) {
	ltv.lock.Lock()
	defer ltv.lock.Unlock()

	ltv.conn = conn
}

// OnConnect 获取连接成功回调函数
func (ltv *LTVClient) OnConnect() func(conn network.IConnection) {
	ltv.lock.Lock()
	defer ltv.lock.Unlock()

	return ltv.onConnect
}

// SetOnConnect 设置连接成功回调函数
func (ltv *LTVClient) SetOnConnect(fn func(conn network.IConnection)) {
	ltv.lock.Lock()
	defer ltv.lock.Unlock()

	ltv.onConnect = fn
}

// OnDisconnect 设置断线回调函数
func (ltv *LTVClient) OnDisconnect() func(conn network.IConnection) {
	ltv.lock.Lock()
	defer ltv.lock.Unlock()

	return ltv.onDisconnect
}

// SetOnDisconnect 设置断线回调函数
func (ltv *LTVClient) SetOnDisconnect(fn func(conn network.IConnection)) {
	ltv.lock.Lock()
	defer ltv.lock.Unlock()

	ltv.onDisconnect = fn
}

// ProtocolCoder 获取协议编码器
func (ltv *LTVClient) ProtocolCoder() network.IProtocolCoder {
	ltv.lock.Lock()
	defer ltv.lock.Unlock()

	return ltv.protocolCoder
}

// HeartbeatFunc 获取心跳函数
func (ltv *LTVClient) HeartbeatFunc() func(conn network.IConnection) {
	ltv.lock.Lock()
	defer ltv.lock.Unlock()

	return ltv.heartbeatFunc
}

// SetHeartbeatFunc 设置心跳函数
func (ltv *LTVClient) SetHeartbeatFunc(fn func(conn network.IConnection)) {
	ltv.lock.Lock()
	defer ltv.lock.Unlock()

	ltv.heartbeatFunc = fn
}

// GetDispatchMsg 获取消息分发函数
func (ltv *LTVClient) GetDispatchMsg() func(conn network.IConnection, packet network.IPacket) {
	ltv.lock.Lock()
	defer ltv.lock.Unlock()

	return ltv.dispatchFunc
}

// SetDispatchMsg 设置消息分发函数
func (ltv *LTVClient) SetDispatchMsg(fn func(conn network.IConnection, packet network.IPacket)) {
	ltv.lock.Lock()
	defer ltv.lock.Unlock()

	ltv.dispatchFunc = fn
}

// GetUrl 获取url
func (ltv *LTVClient) GetUrl() *url.URL {
	ltv.lock.Lock()
	defer ltv.lock.Unlock()

	return ltv.URL
}

// SetUrl 设置url
func (ltv *LTVClient) SetUrl(url *url.URL) {
	ltv.lock.Lock()
	defer ltv.lock.Unlock()

	ltv.URL = url
}

// ===============================================================================
// 私有接口
// ===============================================================================

// run 客户端主循环
func (ltv *LTVClient) run() {
	slog.Info("[LTVClient] Run main loop...", "IP", ltv.cliConf.IP, "Port", ltv.cliConf.Port, "Mode", ltv.cliConf.Mode)

	var dealConn network.IConnection
	switch ltv.cliConf.Mode {
	case NetMode_Websocket:
		wsAddr := fmt.Sprintf("ws://%s:%d", ltv.cliConf.IP, ltv.cliConf.Port)
		if ltv.URL != nil {
			wsAddr = ltv.URL.String()
		}
		// 创建原始的websocket连接
		wsConn, _, err := ltv.wsDialer.Dial(wsAddr, nil)
		if err != nil {
			slog.Error("[LTVClient] Run websocket dial error", "IP", ltv.cliConf.IP, "Port",
				ltv.cliConf.Port, "Mode", ltv.cliConf.Mode, "err", err)
			ltv.ErrChan <- err
			return
		}
		dealConn = newLTVClientWebsocketConnection(ltv, wsConn, ltv.cliConf.Connection)
	default:
		// TCP
		addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", ltv.cliConf.IP, ltv.cliConf.Port))
		if err != nil {
			slog.Error("[LTVClient] Run resolve tcp addr error", "IP", ltv.cliConf.IP, "Port", ltv.cliConf.Port)
			ltv.ErrChan <- err
			return
		}
		tcpConn, err := net.DialTCP("tcp", nil, addr)
		if err != nil {
			slog.Error("[LTVClient] Run dial tcp error", "IP", ltv.cliConf.IP, "Port", ltv.cliConf.Port, "err", err)
			ltv.ErrChan <- err
			return
		}
		dealConn = newLTVClientConnection(ltv, tcpConn, ltv.cliConf.Connection)
	}
	// 设置客户端连接
	ltv.SetConnection(dealConn)
	slog.Info("[LTVClient] Run init client success...", "remoteAddr", dealConn.RemoteAddrString(), "localAddr", dealConn.LocalAddrString())
	dealConn.Start()

	select {
	case <-ltv.ctx.Done():
		// 等待退出
		slog.Info("[LTVClient] Run main loop exit...", "IP", ltv.cliConf.IP, "Port", ltv.cliConf.Port, "Mode", ltv.cliConf.Mode)
		return
	}
}

// ===============================================================================
// 实例化方法
// ===============================================================================

// NewLTVClient 创建ltv客户端
// @param ip ip地址
// @param port 端口
// @param mode 模式
// @param isLittleEndian 是否小端模式
// @return *LTVClient
func NewLTVClient(cliConf *LTVClientConfig, opts ...options.Option[LTVClient]) *LTVClient {
	instance := &LTVClient{
		ErrChan:       make(chan error),
		cliConf:       cliConf,
		protocolCoder: NewLTVProtocolCoder(cliConf.UsedLittleEndian),
		lock:          sync.Mutex{},
		waitGroup:     sync.WaitGroup{},
	}

	// 设置协议模式
	if instance.cliConf.Mode == NetMode_Websocket {
		instance.wsDialer = &websocket.Dialer{}
	}

	for _, opt := range opts {
		opt.Apply(instance)
	}

	return instance
}
