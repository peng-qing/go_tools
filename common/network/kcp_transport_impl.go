package network

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	kcpgo "github.com/xtaci/kcp-go/v5"
)

var (
	_ Transport = (*KcpTransportImpl)(nil)
	_ Listener  = (*KcpListenerImpl)(nil)
	_ Dialer    = (*KcpDialerImpl)(nil)
)

// KcpTransportImpl 是 KCP 传输层实现
type KcpTransportImpl struct {
	session    *kcpgo.UDPSession
	localAddr  net.Addr
	remoteAddr net.Addr
	closeOnce  sync.Once
	closeErr   error
	config     *KcpTransportConfig
}

// newKcpTransportImpl 创建 KCP 传输层实现实例
func newKcpTransportImpl(session *kcpgo.UDPSession, config *KcpTransportConfig) (*KcpTransportImpl, error) {
	// 设置mtu
	if config.Protocol.MTU > 0 && !session.SetMtu(config.Protocol.MTU) {
		return nil, fmt.Errorf("network/kcp: third-party library rejected MTU %d", config.Protocol.MTU)
	}
	// 设置窗口大小
	session.SetWindowSize(config.Protocol.SendWindow, config.Protocol.ReceiveWindow)
	// 设置低延迟模式
	nodelay := 0
	if config.Protocol.NoDelay {
		nodelay = 1
	}
	// 设置拥塞窗口控制
	noCongestion := 0
	if config.Protocol.NoCongestion {
		noCongestion = 1
	}
	session.SetNoDelay(nodelay, int(config.Protocol.Interval), config.Protocol.FastResend, noCongestion)
	session.SetACKNoDelay(config.Protocol.AckNoDelay)
	session.SetWriteDelay(config.Protocol.WriteDelay)

	return &KcpTransportImpl{
		session:    session,
		localAddr:  session.LocalAddr(),
		remoteAddr: session.RemoteAddr(),
		closeErr:   nil,
		config:     config,
	}, nil
}

// Read 读取数据
func (kt *KcpTransportImpl) Read() ([]byte, error) {
	buffer := make([]byte, kt.config.ReadBufferSize)
	if kt.config.IO.ReadTimeout > 0 {
		if err := kt.session.SetReadDeadline(time.Now().Add(kt.config.IO.ReadTimeout)); err != nil {
			return nil, err
		}
	}
	nBytes, readErr := kt.session.Read(buffer)
	if kt.config.IO.ReadTimeout > 0 {
		readErr = errors.Join(readErr, kt.session.SetReadDeadline(time.Time{}))
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
func (kt *KcpTransportImpl) Write(data []byte) error {
	if kt.config.IO.WriteTimeout > 0 {
		if err := kt.session.SetWriteDeadline(time.Now().Add(kt.config.IO.WriteTimeout)); err != nil {
			return err
		}
	}
	var writeErr error
	for len(data) > 0 {
		n, err := kt.session.Write(data)
		if err != nil {
			writeErr = err
			break
		}
		if n <= 0 {
			writeErr = io.ErrShortWrite
			break
		}
		data = data[n:]
	}
	var resetErr error
	if kt.config.IO.WriteTimeout > 0 {
		resetErr = kt.session.SetWriteDeadline(time.Time{})
	}
	if writeErr != nil {
		return writeErr
	}
	return resetErr
}

// Close 关闭传输层
func (kt *KcpTransportImpl) Close() error {
	kt.closeOnce.Do(func() {
		kt.closeErr = kt.session.Close()
	})
	return kt.closeErr
}

// LocalAddr 返回传输层的本地地址
func (kt *KcpTransportImpl) LocalAddr() net.Addr {
	return kt.localAddr
}

// RemoteAddr 返回传输层的远程地址
func (kt *KcpTransportImpl) RemoteAddr() net.Addr {
	return kt.remoteAddr
}

// Conv 返回传输层的会话ID
func (kt *KcpTransportImpl) Conv() uint32 {
	return kt.session.GetConv()
}

// KcpListenerImpl 是 KCP 监听器实现
type KcpListenerImpl struct {
	listener  *kcpgo.Listener     // 底层KCP监听器
	config    *KcpTransportConfig // 配置
	closeOnce sync.Once           // 关闭Once
	closeErr  error               // 关闭错误
}

// NewKcpListener 创建 KCP 监听器
func NewKcpListener(address string, config *KcpTransportConfig) (*KcpListenerImpl, error) {
	block, err := newBlockCrypt(&config.Encryption)
	if err != nil {
		return nil, err
	}
	listener, err := kcpgo.ListenWithOptions(
		address,
		block,
		config.FEC.DataShards,
		config.FEC.ParityShards,
	)
	if err != nil {
		return nil, err
	}
	if config.Socket.DSCP != 0 {
		if err := listener.SetDSCP(config.Socket.DSCP); err != nil {
			_ = listener.Close()
			return nil, fmt.Errorf("network/kcp: set listener DSCP: %w", err)
		}
	}
	if config.Socket.SocketReadBuffer > 0 {
		if err := listener.SetReadBuffer(config.Socket.SocketReadBuffer); err != nil {
			_ = listener.Close()
			return nil, fmt.Errorf("network/kcp: set listener socket read buffer: %w", err)
		}
	}
	if config.Socket.SocketWriteBuffer > 0 {
		if err := listener.SetWriteBuffer(config.Socket.SocketWriteBuffer); err != nil {
			_ = listener.Close()
			return nil, fmt.Errorf("network/kcp: set listener socket write buffer: %w", err)
		}
	}
	return &KcpListenerImpl{listener: listener, config: config, closeErr: nil}, nil
}

// newBlockCrypt 创建 KCP 块加密器
func newBlockCrypt(c *KcpEncryptionConfig) (kcpgo.BlockCrypt, error) {
	switch c.Mode {
	case KCPEncryptionNone:
		return nil, nil
	case KCPEncryptionAES:
		return kcpgo.NewAESBlockCrypt(c.Key)
	default:
		return nil, fmt.Errorf("network/kcp: unsupported encryption %q", c.Mode)
	}
}

// Accept 接受并返回一个Transport 返回Transport时必须已经达到可供Session使用的状态
func (kl *KcpListenerImpl) Accept() (Transport, error) {
	session, err := kl.listener.AcceptKCP()
	if err != nil {
		return nil, err
	}
	transport, err := newKcpTransportImpl(session, kl.config)
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	return transport, nil
}

// ListenerAddr 返回监听器地址
func (kl *KcpListenerImpl) ListenerAddr() net.Addr {
	return kl.listener.Addr()
}

// Close 关闭监听器
func (kl *KcpListenerImpl) Close() error {
	kl.closeOnce.Do(func() {
		kl.closeErr = kl.listener.Close()
	})
	return kl.closeErr
}

// KcpDialerImpl 是 KCP 拨号器实现
type KcpDialerImpl struct {
	config *KcpTransportConfig
}

// NewKcpDialer 创建 KCP 拨号器
func NewKcpDialer(config *KcpTransportConfig) (*KcpDialerImpl, error) {
	return &KcpDialerImpl{config: config}, nil
}

// Dial 拨号
func (kd *KcpDialerImpl) Dial(ctx context.Context, address string) (Transport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 0 || port > 65535 {
		return nil, fmt.Errorf("network/kcp: invalid port %q", portText)
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, errors.New("network/kcp: address resolved to no IP")
	}
	remote := &net.UDPAddr{IP: addresses[0].IP, Port: port, Zone: addresses[0].Zone}
	network := "udp6"
	if remote.IP.To4() != nil {
		network = "udp4"
	}
	block, err := newBlockCrypt(&kd.config.Encryption)
	if err != nil {
		return nil, err
	}
	// conv 是 KCP 会话标识，不要求全局唯一
	// 随机生成以降低远端地址复用时与旧会话重复的概率
	var conv uint32
	if err := binary.Read(rand.Reader, binary.LittleEndian, &conv); err != nil {
		return nil, err
	}
	packetConn, err := net.ListenUDP(network, nil)
	if err != nil {
		return nil, err
	}

	session, err := kcpgo.NewConn4(
		conv,
		remote,
		block,
		kd.config.FEC.DataShards,
		kd.config.FEC.ParityShards,
		true, // ownConn：Session 负责关闭 packetConn
		packetConn,
	)
	if err != nil {
		_ = packetConn.Close()
		return nil, err
	}
	// 客户端 Session 持有独立 socket，在这里应用 socket 配置
	if kd.config.Socket.DSCP != 0 {
		if err := session.SetDSCP(kd.config.Socket.DSCP); err != nil {
			_ = session.Close()
			return nil, fmt.Errorf("network/kcp: set session DSCP: %w", err)
		}
	}
	if kd.config.Socket.SocketReadBuffer > 0 {
		if err := session.SetReadBuffer(kd.config.Socket.SocketReadBuffer); err != nil {
			_ = session.Close()
			return nil, fmt.Errorf("network/kcp: set session socket read buffer: %w", err)
		}
	}
	if kd.config.Socket.SocketWriteBuffer > 0 {
		if err := session.SetWriteBuffer(kd.config.Socket.SocketWriteBuffer); err != nil {
			_ = session.Close()
			return nil, fmt.Errorf("network/kcp: set session socket write buffer: %w", err)
		}
	}
	// 创建 Transport 实例
	transport, err := newKcpTransportImpl(session, kd.config)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	return transport, nil
}
