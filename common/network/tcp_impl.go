package network

import (
	"context"
	"fmt"
	"net"
	"time"
)

var (
	_ Transport = (*TCPTransportImpl)(nil) // Transport TCP 实现
	_ Listener  = (*TCPListenerImpl)(nil)  // Listener TCP 实现
	_ Dialer    = (*TCPDialerImpl)(nil)    // Dialer TCP 实现
)

// TCPTransportImpl 是基于 TCP 的 Transport 实现
type TCPTransportImpl struct {
	conn    *net.TCPConn // 底层tcp连接
	tcpConf TCPConfig    // tcp 连接配置
}

// newTCPTransportImpl 创建一个TCP Transport 实例
// 调用构造函数时 conn 的所有权移交给 transport, 失败由transport关闭
func newTCPTransportImpl(conn *net.TCPConn, tcpConf TCPConfig) (transport *TCPTransportImpl, err error) {
	if conn == nil {
		return nil, fmt.Errorf("network/tcp: nil TCP connection")
	}
	defer func() {
		// 创建失败关闭socket
		if err != nil {
			// 不覆盖err 保留原始错误
			_ = conn.Close()
		}
	}()

	if tcpConf.KeepalivePeriod > 0 {
		if err = conn.SetKeepAlive(true); err != nil {
			return nil, err
		}
		if err = conn.SetKeepAlivePeriod(tcpConf.KeepalivePeriod); err != nil {
			return nil, err
		}
	}
	if tcpConf.ReadBufferSize > 0 {
		if err = conn.SetReadBuffer(tcpConf.ReadBufferSize); err != nil {
			return nil, err
		}
	}
	if tcpConf.WriteBufferSize > 0 {
		if err = conn.SetWriteBuffer(tcpConf.WriteBufferSize); err != nil {
			return nil, err
		}
	}
	if err = conn.SetNoDelay(tcpConf.NoDelay); err != nil {
		return nil, err
	}

	return &TCPTransportImpl{conn: conn, tcpConf: tcpConf}, nil
}

// Read 读取数据
func (t *TCPTransportImpl) Read(buffer []byte) (int, error) {
	if len(buffer) <= 0 {
		// 保证和标准 io.Reader 一致 允许传入空buffer 返回0,nil
		return 0, nil
	}
	if t.tcpConf.ReadTimeout > 0 {
		if err := t.conn.SetReadDeadline(time.Now().Add(t.tcpConf.ReadTimeout)); err != nil {
			return 0, err
		}
		defer func() {
			_ = t.conn.SetReadDeadline(time.Time{})
		}()
	}
	// 交给底层socket处理
	return t.conn.Read(buffer)
}

// Write 写入数据
func (t *TCPTransportImpl) Write(data []byte) (int, error) {
	if len(data) <= 0 {
		// 保证和标准 io.Writer 一致 允许传入空buffer 返回0,nil
		return 0, nil
	}
	if t.tcpConf.WriteTimeout > 0 {
		if err := t.conn.SetWriteDeadline(time.Now().Add(t.tcpConf.WriteTimeout)); err != nil {
			return 0, err
		}
		defer func() {
			_ = t.conn.SetWriteDeadline(time.Time{})
		}()
	}
	// 交给底层socket处理
	return t.conn.Write(data)
}

// Close 关闭连接
func (t *TCPTransportImpl) Close() error {
	return t.conn.Close()
}

// LocalAddr 本地地址
func (t *TCPTransportImpl) LocalAddr() net.Addr {
	return t.conn.LocalAddr()
}

// RemoteAddr 对端地址
func (t *TCPTransportImpl) RemoteAddr() net.Addr {
	return t.conn.RemoteAddr()
}

// TCPListenerImpl 基于 TCP 的 Listener 实现
type TCPListenerImpl struct {
	listener *net.TCPListener // 底层TCP Listener
	tcpConf  TCPConfig        // 配置
}

// NewTCPListener 创建TCP Listener
func NewTCPListener(address string, tcpConf TCPConfig) (*TCPListenerImpl, error) {
	addr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return nil, err
	}
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &TCPListenerImpl{listener: listener, tcpConf: tcpConf}, nil
}

// Accept 接受一个 Transport 连接
func (ln *TCPListenerImpl) Accept() (Transport, error) {
	conn, err := ln.listener.AcceptTCP()
	if err != nil {
		return nil, err
	}

	return newTCPTransportImpl(conn, ln.tcpConf)
}

// Close 关闭监听器
func (ln *TCPListenerImpl) Close() error {
	return ln.listener.Close()
}

// Addr 返回监听器的地址
func (ln *TCPListenerImpl) Addr() net.Addr {
	return ln.listener.Addr()
}

// TCPDialerImpl TCP Dialer 客户端拨号实现
type TCPDialerImpl struct {
	dialer  net.Dialer // 底层 TCP 拨号器
	tcpConf TCPConfig  // 配置
}

// NewTCPDialer 创建一个 TCP 拨号器
func NewTCPDialer(tcpConf TCPConfig) *TCPDialerImpl {
	return &TCPDialerImpl{tcpConf: tcpConf}
}

// DialContext 拨号
func (d *TCPDialerImpl) DialContext(ctx context.Context, address string) (Transport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	conn, err := d.dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		_ = conn.Close()
		return nil, fmt.Errorf("network/tcp: dial di not return TCP connection")
	}
	return newTCPTransportImpl(tcpConn, d.tcpConf)
}
