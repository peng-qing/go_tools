package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/peng-qing/go_tools/common/container"
)

var (
	_ Transport = (*WebsocketTransportImpl)(nil)
	_ Listener  = (*WebsocketListenerImpl)(nil)
	_ Dialer    = (*WebsocketDialerImpl)(nil)
)

var (
	// ErrNonBinaryMessage 非二进制消息错误
	ErrNonBinaryMessage = errors.New("newnet/websocket: non-binary message")
)

// WebsocketTransportImpl 是 WebSocket 传输层实现
type WebsocketTransportImpl struct {
	conn       *websocket.Conn           // 底层websocket连接
	localAddr  net.Addr                  // 本地地址
	remoteAddr net.Addr                  // 远程地址
	closeOnce  sync.Once                 // 关闭Once
	closeErr   error                     // 关闭错误
	config     *WebsocketTransportConfig // 配置
}

// newWebsocketTransportImpl 创建 WebSocket 传输层实现实例
func newWebsocketTransportImpl(conn *websocket.Conn, config *WebsocketTransportConfig) *WebsocketTransportImpl {
	if config.MaxMessageSize > 0 {
		conn.SetReadLimit(config.MaxMessageSize)
	}
	return &WebsocketTransportImpl{
		conn:       conn,
		localAddr:  conn.LocalAddr(),
		remoteAddr: conn.RemoteAddr(),
		config:     config,
	}
}

// Read 读取数据
func (w *WebsocketTransportImpl) Read() ([]byte, error) {
	if w.config.IO.ReadTimeout > 0 {
		if err := w.conn.SetReadDeadline(time.Now().Add(w.config.IO.ReadTimeout)); err != nil {
			return nil, err
		}
	}
	msgType, data, readErr := w.conn.ReadMessage()
	var resetErr error
	if w.config.IO.ReadTimeout > 0 {
		resetErr = w.conn.SetReadDeadline(time.Time{})
	}
	if readErr != nil {
		return nil, readErr
	}
	if msgType != websocket.BinaryMessage {
		return nil, fmt.Errorf("%w: type %d", ErrNonBinaryMessage, msgType)
	}
	return data, resetErr
}

// Write 写入数据
func (w *WebsocketTransportImpl) Write(data []byte) error {
	if w.config.IO.WriteTimeout > 0 {
		if err := w.conn.SetWriteDeadline(time.Now().Add(w.config.IO.WriteTimeout)); err != nil {
			return err
		}
	}
	var resetErr error
	writeErr := w.conn.WriteMessage(websocket.BinaryMessage, data)
	if w.config.IO.WriteTimeout > 0 {
		resetErr = w.conn.SetWriteDeadline(time.Time{})
	}
	if writeErr != nil {
		return writeErr
	}
	return resetErr
}

// Close 关闭连接
func (w *WebsocketTransportImpl) Close() error {
	w.closeOnce.Do(func() {
		w.closeErr = w.conn.Close()
	})
	return w.closeErr
}

// LocalAddr 返回本地地址
func (w *WebsocketTransportImpl) LocalAddr() net.Addr {
	return w.localAddr
}

// RemoteAddr 返回远程地址
func (w *WebsocketTransportImpl) RemoteAddr() net.Addr {
	return w.remoteAddr
}

// WebsocketListenerImpl 是 WebSocket 监听器实现
type WebsocketListenerImpl struct {
	listener     net.Listener                 // 底层监听器
	server       *http.Server                 // http服务器
	upgrader     *websocket.Upgrader          // 协议升级
	acceptQueue  chan *WebsocketTransportImpl // 传输层接受队列
	upgradeSlots chan container.None          // 协议升级槽位
	upgradeWg    sync.WaitGroup               // 协议升级等待组
	acceptErr    error                        // 接受错误 在close(done)前写入 只读, done广播并同步终止原因
	done         chan container.None          // 停止通道
	closeMu      sync.Mutex                   // 关闭互斥锁
	closed       bool                         // 是否关闭
	closeErr     error                        // 关闭错误
	config       *WebsocketListenerConfig     // 配置
}

func NewWebsocketListener(config *WebsocketListenerConfig) (*WebsocketListenerImpl, error) {
	if config.Path == "" {
		config.Path = "/"
	}
	if config.CheckOrigin == nil {
		config.CheckOrigin = func(r *http.Request) bool {
			return true
		}
	}
	listener, err := net.Listen("tcp", config.Address)
	if err != nil {
		return nil, err
	}
	wsImpl := &WebsocketListenerImpl{
		config:       config,
		listener:     listener,
		acceptQueue:  make(chan *WebsocketTransportImpl, config.MaxPendingConnections),
		upgradeSlots: make(chan container.None, config.MaxConcurrentUpgrades),
		done:         make(chan container.None),
	}
	wsImpl.upgrader = &websocket.Upgrader{
		ReadBufferSize:    config.Transport.ReadBufferSize,
		WriteBufferSize:   config.Transport.WriteBufferSize,
		HandshakeTimeout:  config.Transport.Handshake.Timeout,
		EnableCompression: config.Transport.Handshake.EnableCompression,
		CheckOrigin:       config.CheckOrigin,
	}
	httpMux := http.NewServeMux()
	httpMux.HandleFunc(config.Path, wsImpl.handleUpgrade)
	wsImpl.server = &http.Server{
		Handler:           httpMux,
		ReadHeaderTimeout: config.Transport.Handshake.Timeout,
		MaxHeaderBytes:    config.MaxHeaderBytes,
	}
	go wsImpl.serve()
	return wsImpl, nil
}

// 启动HTTP服务
func (wl *WebsocketListenerImpl) serve() {
	err := wl.server.Serve(wl.listener)
	if errors.Is(err, http.ErrServerClosed) {
		err = ErrClosed
	}
	_ = wl.closeWithCause(err)
}

// 主动关闭和 Serve 失败共用清理流程，先发生的关闭决定 Accept 终止原因。
func (wl *WebsocketListenerImpl) closeWithCause(cause error) error {
	wl.closeMu.Lock()
	defer wl.closeMu.Unlock()
	if wl.closed {
		return wl.closeErr
	}
	wl.closed = true
	wl.acceptErr = cause
	close(wl.done)

	wl.closeErr = wl.server.Close()
	if err := wl.listener.Close(); wl.closeErr == nil && !errors.Is(err, net.ErrClosed) {
		wl.closeErr = err
	}
	// HTTP Close 不管理已 Hijack 的连接。等待升级处理结束后再清空队列，
	// 防止清理完成后仍有升级连接入队
	wl.upgradeWg.Wait()

	for {
		select {
		case transport := <-wl.acceptQueue:
			// 关闭传输层
			_ = transport.Close()
		default:
			return wl.closeErr
		}
	}
}

// 处理HTTP升级请求
func (wl *WebsocketListenerImpl) handleUpgrade(w http.ResponseWriter, r *http.Request) {
	wl.closeMu.Lock()
	if wl.closed {
		wl.closeMu.Unlock()
		// 拒绝请求
		http.Error(w, "listener closed", http.StatusServiceUnavailable)
		return
	}
	wl.upgradeWg.Add(1)
	wl.closeMu.Unlock()

	defer wl.upgradeWg.Done()

	select {
	case wl.upgradeSlots <- container.None{}:
		// 获取升级槽位
		defer func() { <-wl.upgradeSlots }()
	default:
		// 超限直接拒绝
		http.Error(w, "too many concurrent upgrades", http.StatusServiceUnavailable)
		return
	}
	// 链接升级
	conn, err := wl.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// 升级失败结束本次请求
		return
	}
	// 创建传输层实例
	transport := newWebsocketTransportImpl(conn, wl.config.Transport)
	select {
	case wl.acceptQueue <- transport:
	case <-wl.done:
		// 关闭尚未交付的连接
		_ = transport.Close()
	default:
		// Upgrade 已完成但 Accept 队列已满，立即释放连接而不是阻塞 HTTP handler。
		_ = transport.Close()
	}
}

// Accept 接受连接
func (wl *WebsocketListenerImpl) Accept() (Transport, error) {
	var transport *WebsocketTransportImpl

	// 等待连接或终止
	select {
	case <-wl.done:
		return nil, wl.acceptErr
	case transport = <-wl.acceptQueue:
	}

	// 队列和关闭通知可能同时就绪 再确认
	select {
	case <-wl.done:
		// 关闭尚未交付的连接
		_ = transport.Close()
		return nil, wl.acceptErr
	default:
		return transport, nil
	}
}

// Addr 返回监听器地址
func (wl *WebsocketListenerImpl) ListenerAddr() net.Addr {
	return wl.listener.Addr()
}

func (wl *WebsocketListenerImpl) Close() error {
	return wl.closeWithCause(ErrClosed)
}

// WebsocketDialerImpl 是 WebSocket 拨号器实现
type WebsocketDialerImpl struct {
	dialer *websocket.Dialer
	config *WebsocketTransportConfig
}

// NewWebsocketDialer 创建 WebSocket 拨号器实例
func NewWebsocketDialer(config *WebsocketTransportConfig) *WebsocketDialerImpl {
	return &WebsocketDialerImpl{
		config: config,
		dialer: &websocket.Dialer{
			ReadBufferSize:    config.ReadBufferSize,
			WriteBufferSize:   config.WriteBufferSize,
			HandshakeTimeout:  config.Handshake.Timeout,
			EnableCompression: config.Handshake.EnableCompression,
		},
	}
}

// Dial 拨号连接
func (wd *WebsocketDialerImpl) Dial(ctx context.Context, address string) (Transport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	parsed, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "ws" && parsed.Scheme != "wss" {
		return nil, errors.New("network/websocket: address must use ws or wss")
	}
	conn, response, err := wd.dialer.DialContext(ctx, address, nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	return newWebsocketTransportImpl(conn, wd.config), nil
}
