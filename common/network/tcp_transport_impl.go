package network

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

var (
	_ Transport = (*TcpTransportImpl)(nil)
	_ Listener  = (*TcpListenerImpl)(nil)
	_ Dialer    = (*TcpDialerImpl)(nil)
)

type TcpTransportImpl struct {
	conn       *net.TCPConn        // 底层网络连接
	localAddr  net.Addr            // 本地地址
	remoteAddr net.Addr            // 远程地址
	closeOnce  sync.Once           // 关闭Once
	closeErr   error               // 关闭错误
	config     *TcpTransportConfig // 配置
}

// newTcpTransportImpl 创建 TCP 传输层实现实例
func newTcpTransportImpl(conn *net.TCPConn, config *TcpTransportConfig) (*TcpTransportImpl, error) {
	if config.KeepAlive != nil {
		if err := conn.SetKeepAliveConfig(*config.KeepAlive); err != nil {
			return nil, err
		}
	}
	if config.NoDelay {
		if err := conn.SetNoDelay(true); err != nil {
			return nil, err
		}
	}
	return &TcpTransportImpl{
		conn:       conn,
		config:     config,
		localAddr:  conn.LocalAddr(),
		remoteAddr: conn.RemoteAddr(),
	}, nil
}

// Read 读取数据
func (t *TcpTransportImpl) Read() ([]byte, error) {
	buffer := make([]byte, t.config.ReadBufferSize)
	if t.config.IO.ReadTimeout > 0 {
		if err := t.conn.SetReadDeadline(time.Now().Add(t.config.IO.ReadTimeout)); err != nil {
			return nil, err
		}
	}
	nBytes, readErr := t.conn.Read(buffer)
	if t.config.IO.ReadTimeout > 0 {
		readErr = errors.Join(readErr, t.conn.SetReadDeadline(time.Time{}))
	}
	if nBytes > 0 {
		return buffer[:nBytes], readErr
	}
	if readErr != nil {
		return nil, readErr
	}
	return nil, io.ErrNoProgress
}

// Write 写入数据
func (t *TcpTransportImpl) Write(data []byte) error {
	if t.config.IO.WriteTimeout > 0 {
		if err := t.conn.SetWriteDeadline(time.Now().Add(t.config.IO.WriteTimeout)); err != nil {
			return err
		}
	}
	var writeErr error
	for len(data) > 0 {
		nBytes, err := t.conn.Write(data)
		if err != nil {
			writeErr = err
			break
		}
		if nBytes <= 0 {
			writeErr = io.ErrShortWrite
			break
		}
		data = data[nBytes:]
	}
	var resetErr error
	if t.config.IO.WriteTimeout > 0 {
		resetErr = t.conn.SetWriteDeadline(time.Time{})
	}
	if writeErr != nil {
		return writeErr
	}
	return resetErr
}

// Close 关闭传输层
func (t *TcpTransportImpl) Close() error {
	t.closeOnce.Do(func() {
		t.closeErr = t.conn.Close()
	})
	return t.closeErr
}

// LocalAddr 返回本地地址
func (t *TcpTransportImpl) LocalAddr() net.Addr { return t.localAddr }

// RemoteAddr 返回远程地址
func (t *TcpTransportImpl) RemoteAddr() net.Addr { return t.remoteAddr }

// TcpListenerImpl 是TCP监听器的实现
type TcpListenerImpl struct {
	listener  *net.TCPListener    // 底层TCP监听器
	closeOnce sync.Once           // 关闭Once
	closeErr  error               // 关闭错误
	config    *TcpTransportConfig // 配置
}

// NewTcpListener 创建TCP监听器
func NewTcpListener(address string, config *TcpTransportConfig) (*TcpListenerImpl, error) {
	addr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return nil, err
	}
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &TcpListenerImpl{listener: listener, config: config}, nil
}

// Accept 接受并返回一个Transport 返回Transport时必须已经达到可供Session使用的状态
func (l *TcpListenerImpl) Accept() (Transport, error) {
	conn, err := l.listener.AcceptTCP()
	if err != nil {
		return nil, err
	}
	transport, err := newTcpTransportImpl(conn, l.config)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return transport, nil
}

// Close 关闭Listener 必须解除阻塞的Accept 需要避免一个对端的可控输入直接终止整个服务
func (l *TcpListenerImpl) Close() error {
	l.closeOnce.Do(func() {
		l.closeErr = l.listener.Close()
	})
	return l.closeErr
}

// Addr 返回Listener的地址
func (l *TcpListenerImpl) ListenerAddr() net.Addr {
	return l.listener.Addr()
}

// TcpDialerImpl 是TCP拨号器的实现
type TcpDialerImpl struct {
	dialer net.Dialer          // 底层TCP拨号器
	config *TcpTransportConfig // 配置
}

// NewTcpDialer 创建TCP拨号器
func NewTcpDialer(config *TcpTransportConfig) *TcpDialerImpl {
	return &TcpDialerImpl{config: config}
}

// Dial 拨号
func (d *TcpDialerImpl) Dial(ctx context.Context, address string) (Transport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := d.dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		_ = conn.Close()
		return nil, errors.New("network/tcp: dial did not return TCP connection")
	}
	transport, err := newTcpTransportImpl(tcpConn, d.config)
	if err != nil {
		_ = tcpConn.Close()
		return nil, err
	}
	return transport, nil
}
