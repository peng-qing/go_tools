package network

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/peng-qing/go_tools/common/container"
)

var (
	_ Transport = (*WsTransportImpl)(nil) // Transport websocket 实现
	_ Listener  = (*WsListenerImpl)(nil)  // Listener websocket 实现
	_ Dialer    = (*WsDialerImpl)(nil)    // Dialer websocket 实现
)

const (
	// 已经完成协议升级、等待 Accept 交付的默认最大连接数
	defaultWsMaxPendingConnections = 1024
	// 默认握手超时
	defaultWsHandshakeTimeout = 10 * time.Second
	// 请求头读取超时
	defaultWsReadHeaderTimeout = 10 * time.Second
	// 空闲连接超时
	defaultWsIdleTimeout = 60 * time.Second
)

// WsTransportImpl websocket transport 实现
type WsTransportImpl struct {
	conn   *websocket.Conn // 底层 websocket 连接
	wsConf WsConfig        // websocket 配置
	reader io.Reader       // 报错websocket 二进制未读取完成的呢日 将ws消息适配字节流格式
}

// newWsTransportImpl 创建一个底层未websocket的 Transport 示例
func newWsTransportImpl(conn *websocket.Conn, wsConf WsConfig) *WsTransportImpl {
	if wsConf.MaxMessageSize > 0 {
		conn.SetReadLimit(wsConf.MaxMessageSize)
	}
	return &WsTransportImpl{conn: conn, wsConf: wsConf}
}

// Read 读取数据
func (w *WsTransportImpl) Read(buffer []byte) (int, error) {
	if len(buffer) <= 0 {
		return 0, nil
	}
	if w.wsConf.ReadTimeout > 0 {
		if err := w.conn.SetReadDeadline(time.Now().Add(w.wsConf.ReadTimeout)); err != nil {
			return 0, err
		}
		defer func() {
			_ = w.conn.SetReadDeadline(time.Time{})
		}()
	}
	for {
		if w.reader == nil {
			msgType, reader, err := w.conn.NextReader()
			if err != nil {
				return 0, err
			}
			if msgType != websocket.BinaryMessage {
				return 0, fmt.Errorf("network/ws: expected binary message, got: %d", msgType)
			}
			w.reader = reader
		}
		nBytes, err := w.reader.Read(buffer)
		if errors.Is(err, io.EOF) {
			// 当前消息读取完成
			w.reader = nil
			if nBytes > 0 {
				// 隐藏websocket消息边界
				return nBytes, nil
			}
			continue
		}
		return nBytes, err
	}
}

// Write 写入数据
func (w *WsTransportImpl) Write(data []byte) (int, error) {
	if len(data) <= 0 {
		return 0, nil
	}
	if w.wsConf.WriteTimeout > 0 {
		if err := w.conn.SetWriteDeadline(time.Now().Add(w.wsConf.WriteTimeout)); err != nil {
			return 0, err
		}
		defer func() {
			_ = w.conn.SetWriteDeadline(time.Time{})
		}()
	}
	if err := w.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		// Connection 收到错误后必须终止连接，不能尝试继续发送
		// 当前消息的剩余部分，也不能继续发送下一帧
		return 0, err
	}
	// WriteMessage 不提供准确的部分写入字节数
	return len(data), nil
}

// Close 关闭连接
func (w *WsTransportImpl) Close() error {
	return w.conn.Close()
}

// LocalAddr 返回本地地址
func (w *WsTransportImpl) LocalAddr() net.Addr {
	return w.conn.LocalAddr()
}

// RemoteAddr 返回远程地址
func (w *WsTransportImpl) RemoteAddr() net.Addr {
	return w.conn.RemoteAddr()
}

// WsListenerImpl websocket 服务端监听实现
type WsListenerImpl struct {
	listener     net.Listener          // 底层监听器
	httpServer   *http.Server          // http服务器
	upgrader     *websocket.Upgrader   // 协议升级器
	lnConf       WsListenerConfig      // 配置
	acceptQueue  chan *WsTransportImpl // 无缓冲，仅用于把连接交给 Accept
	pendingSlots chan container.None   // 限制正在握手及等待 Accept 的连接总数
	done         chan container.None   // 停止信号 广播listener生命周期结束
	upgradeWg    sync.WaitGroup        // 协议升级等待组 等待所有upgrade完成
	closeOnce    sync.Once             // 关闭Once
	closeMu      sync.Mutex            // 锁 保护 closed closeErr
	closed       bool                  // 关闭标记
	closeErr     error                 // 关闭原因
	acceptErr    error                 // close(done)前写入 之后只读
}

// NewWsListenerImpl 构造函数 websocket服务端实现
func NewWsListenerImpl(lnConf WsListenerConfig) (*WsListenerImpl, error) {
	if lnConf.WsConf.Path == "" {
		lnConf.WsConf.Path = "/"
	}
	if !strings.HasPrefix(lnConf.WsConf.Path, "/") {
		lnConf.WsConf.Path = "/" + lnConf.WsConf.Path
	}
	if strings.ContainsAny(lnConf.WsConf.Path, "?#") {
		return nil, fmt.Errorf("network/ws: path contains query or fragment: %q", lnConf.WsConf.Path)
	}
	if lnConf.MaxPendingConnections <= 0 {
		lnConf.MaxPendingConnections = defaultWsMaxPendingConnections
	}
	if lnConf.WsConf.HandshakeTimeout <= 0 {
		lnConf.WsConf.HandshakeTimeout = defaultWsHandshakeTimeout
	}
	if lnConf.ReadHeaderTimeout <= 0 {
		lnConf.ReadHeaderTimeout = defaultWsReadHeaderTimeout
	}
	if lnConf.IdleTimeout <= 0 {
		lnConf.IdleTimeout = defaultWsIdleTimeout
	}
	if lnConf.MaxHeaderBytes <= 0 {
		lnConf.MaxHeaderBytes = http.DefaultMaxHeaderBytes
	}
	if lnConf.WsConf.TLSConfig != nil {
		if len(lnConf.WsConf.TLSConfig.Certificates) == 0 &&
			lnConf.WsConf.TLSConfig.GetCertificate == nil &&
			lnConf.WsConf.TLSConfig.GetConfigForClient == nil {
			return nil, errors.New("network/ws: server TLS no certificate configured")
		}
		tlsClone := lnConf.WsConf.TLSConfig.Clone()
		if tlsClone.MinVersion == 0 {
			tlsClone.MinVersion = tls.VersionTLS12
		}
		lnConf.WsConf.TLSConfig = tlsClone
	}
	if len(lnConf.WsConf.Subprotocols) > 0 {
		lnConf.WsConf.Subprotocols = append([]string(nil), lnConf.WsConf.Subprotocols...)
	}
	upgrader := &websocket.Upgrader{
		HandshakeTimeout:  lnConf.WsConf.HandshakeTimeout,
		EnableCompression: lnConf.WsConf.EnableCompression,
		Subprotocols:      lnConf.WsConf.Subprotocols,
	}
	if lnConf.WsConf.ReadBufferSize > 0 {
		upgrader.ReadBufferSize = lnConf.WsConf.ReadBufferSize
	}
	if lnConf.WsConf.WriteBufferSize > 0 {
		upgrader.WriteBufferSize = lnConf.WsConf.WriteBufferSize
	}
	if lnConf.CheckOrigin != nil {
		upgrader.CheckOrigin = lnConf.CheckOrigin
	}
	listener := &WsListenerImpl{
		lnConf:       lnConf,
		upgrader:     upgrader,
		acceptQueue:  make(chan *WsTransportImpl),
		pendingSlots: make(chan container.None, lnConf.MaxPendingConnections),
		done:         make(chan container.None),
	}
	httpMux := http.NewServeMux()
	httpMux.HandleFunc(lnConf.WsConf.Path, listener.handleUpgrade)
	listener.httpServer = &http.Server{
		Handler:           httpMux,
		ReadHeaderTimeout: lnConf.ReadHeaderTimeout,
		IdleTimeout:       lnConf.IdleTimeout,
		MaxHeaderBytes:    lnConf.MaxHeaderBytes,
		TLSConfig:         lnConf.WsConf.TLSConfig,
	}
	ln, err := net.Listen("tcp", lnConf.Address)
	if err != nil {
		return nil, err
	}
	if lnConf.WsConf.TLSConfig != nil {
		ln = tls.NewListener(ln, lnConf.WsConf.TLSConfig)
	}
	listener.listener = ln

	// 启动http服务监听
	go listener.serve()
	return listener, nil
}

// serve 启动HTTP服务
func (ln *WsListenerImpl) serve() {
	err := ln.httpServer.Serve(ln.listener)
	if err == nil || errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		err = net.ErrClosed
	}
	// 关闭原因
	ln.closeWithCause(err)
}

// handleUpgrade  处理WebSocket升级请求
func (ln *WsListenerImpl) handleUpgrade(w http.ResponseWriter, r *http.Request) {
	ln.closeMu.Lock()
	if ln.closed {
		ln.closeMu.Unlock()
		http.Error(w, "listener close", http.StatusServiceUnavailable)
		return
	}
	// Add 与 closed 检查必须在同一把锁内
	// 关闭阶段设置 closed 后，不会再有新的 Add
	ln.upgradeWg.Add(1)
	ln.closeMu.Unlock()
	defer ln.upgradeWg.Done()

	// 容量不足时，在升级前拒绝请求
	select {
	case ln.pendingSlots <- container.None{}:
		defer func() {
			<-ln.pendingSlots
		}()
	default:
		http.Error(w, "too many concurrent upgrades", http.StatusServiceUnavailable)
		return
	}
	wsConn, err := ln.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// 升级失败 结束
		return
	}
	transport := newWsTransportImpl(wsConn, ln.lnConf.WsConf)

	// 持有 slot，直到连接交接完成或 listener 关闭
	select {
	case ln.acceptQueue <- transport:
		// 所有权交给 acceptQueue
	case <-ln.done:
		// 尚未交付的连接由当前 handler 关闭
		_ = transport.Close()
	}
}

// closeWithCause 关闭listener并记录原因
func (ln *WsListenerImpl) closeWithCause(cause error) {
	if cause == nil {
		cause = net.ErrClosed
	}
	ln.closeOnce.Do(func() {
		ln.closeMu.Lock()
		ln.closed = true
		ln.acceptErr = cause
		close(ln.done)
		ln.closeMu.Unlock()

		serverErr := ln.httpServer.Close()
		if errors.Is(serverErr, net.ErrClosed) || errors.Is(serverErr, http.ErrServerClosed) {
			serverErr = nil
		}

		// 同时覆盖 Close 发生在 Serve 启动之前的情况
		listenerErr := ln.listener.Close()
		if errors.Is(listenerErr, net.ErrClosed) || errors.Is(listenerErr, http.ErrServerClosed) {
			listenerErr = nil
		}

		ln.closeErr = errors.Join(serverErr, listenerErr)

		// done 已关闭，等待交接的 handler 会关闭其连接并退出。
		// 正在执行 Upgrade 的 handler 则在 Upgrade 返回后退出。
		ln.upgradeWg.Wait()
	})
}

// Accept 交付一条可用连接
func (ln *WsListenerImpl) Accept() (Transport, error) {
	select {
	case <-ln.done:
		return nil, ln.acceptErr
	case transport := <-ln.acceptQueue:
		// 交接和关闭通知可能同时就绪
		// 如果已观察到关闭，则关闭刚接收的连接
		select {
		case <-ln.done:
			_ = transport.Close()
			return nil, ln.acceptErr
		default:
			return transport, nil
		}
	}
}

// Close 关闭listener
func (ln *WsListenerImpl) Close() error {
	ln.closeWithCause(net.ErrClosed)
	return ln.closeErr
}

// Addr 返回监听器地址
func (ln *WsListenerImpl) Addr() net.Addr {
	return ln.listener.Addr()
}

// WsDialerImpl 客户端拨号实现
type WsDialerImpl struct {
	dialer     *websocket.Dialer // 底层拨号
	dialerConf WsDialerConfig    // 配置
}

// NewWsDialerImpl 创建一个 WebSocket 拨号器实例
func NewWsDialerImpl(dialConf WsDialerConfig) *WsDialerImpl {
	if dialConf.WsConf.Path == "" {
		dialConf.WsConf.Path = "/"
	}
	if !strings.HasPrefix(dialConf.WsConf.Path, "/") {
		dialConf.WsConf.Path = "/" + dialConf.WsConf.Path
	}
	if dialConf.WsConf.HandshakeTimeout <= 0 {
		dialConf.WsConf.HandshakeTimeout = defaultWsHandshakeTimeout
	}
	// TLSConfig 只配置 TLS 参数，不隐含强制 TLS。
	// 客户端无需配置服务端证书；客户端证书由 mTLS 场景按需提供。
	if dialConf.WsConf.TLSConfig != nil {
		tlsClone := dialConf.WsConf.TLSConfig.Clone()
		if tlsClone.MinVersion == 0 {
			tlsClone.MinVersion = tls.VersionTLS12
		}
		dialConf.WsConf.TLSConfig = tlsClone
	}
	if len(dialConf.WsConf.Subprotocols) > 0 {
		dialConf.WsConf.Subprotocols = append([]string(nil), dialConf.WsConf.Subprotocols...)
	}
	dialConf.Header = dialConf.Header.Clone()
	if dialConf.Proxy == nil {
		dialConf.Proxy = http.ProxyFromEnvironment
	}
	dialer := &websocket.Dialer{
		Proxy:             dialConf.Proxy,
		HandshakeTimeout:  dialConf.WsConf.HandshakeTimeout,
		EnableCompression: dialConf.WsConf.EnableCompression,
		TLSClientConfig:   dialConf.WsConf.TLSConfig,
		Subprotocols:      dialConf.WsConf.Subprotocols,
	}
	if dialConf.WsConf.ReadBufferSize > 0 {
		dialer.ReadBufferSize = dialConf.WsConf.ReadBufferSize
	}
	if dialConf.WsConf.WriteBufferSize > 0 {
		dialer.WriteBufferSize = dialConf.WsConf.WriteBufferSize
	}

	return &WsDialerImpl{dialer: dialer, dialerConf: dialConf}
}

// endpoint 解析地址并补充默认协议和路径 并校验连接地址
func (d *WsDialerImpl) endpoint(address string) (string, error) {
	if address == "" {
		return "", errors.New("network/ws: address is empty")
	}
	if address != strings.TrimSpace(address) {
		return "", errors.New("network/ws: address contains surrounding whitespace")
	}
	// WebSocket 地址不允许 fragment，包括末尾空的 "#"。
	// 编码后的 %23 不受影响。
	if strings.Contains(address, "#") {
		return "", errors.New("network/ws: address must not contain fragment")
	}
	// 只识别出现在地址开头部分的协议分隔符
	schemeEnd := strings.Index(address, "://")
	hasScheme := schemeEnd >= 0 && !strings.ContainsAny(address[:schemeEnd], "/?#")
	if !hasScheme {
		scheme := "ws"
		if d.dialerConf.RequireTLS || d.dialerConf.WsConf.TLSConfig != nil {
			scheme = "wss"
		}
		address = scheme + "://" + address
	}

	u, err := url.Parse(address)
	if err != nil {
		// 不回显完整地址 避免错误日志包含查询参数中的凭据
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			// 只保留底层错误
			err = urlErr.Err
		}
		return "", fmt.Errorf("network/ws: parse address: %w", err)
	}

	// 统一校验 和 tls 要求
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return "", errors.New("network/ws: address must use ws or wss")
	}
	if d.dialerConf.RequireTLS && u.Scheme != "wss" {
		return "", errors.New("network/ws: TLS required, address must use wss")
	}
	if u.Hostname() == "" {
		return "", errors.New("network/ws: address must have host")
	}
	if u.Path == "" {
		u.Path = d.dialerConf.WsConf.Path
		u.RawPath = ""
	}
	return u.String(), nil
}

// DialContext websocket客户端拨号
func (d *WsDialerImpl) DialContext(ctx context.Context, address string) (Transport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	endpoint, err := d.endpoint(address)
	if err != nil {
		return nil, err
	}
	wsConn, response, err := d.dialer.DialContext(ctx, endpoint, d.dialerConf.Header)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	return newWsTransportImpl(wsConn, d.dialerConf.WsConf), nil
}
