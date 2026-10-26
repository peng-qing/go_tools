package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/xtaci/kcp-go/v5"
)

var (
	_ Transport = (*KCPTransportImpl)(nil)
	_ Listener  = (*KCPListenerImpl)(nil)
	_ Dialer    = (*KCPDialerImpl)(nil)
)

// KCPTransportImpl 基于 kcp-go UDPSession 的 Transport 实现
// 如果底层的UDP socket 如果被关闭，依赖他的所有 Session 都会失去能力
// 单独关闭上层Session不会关闭共享的 UDP Socket
type KCPTransportImpl struct {
	session *kcp.UDPSession // udp Session
	kcpConf KCPConfig       // 配置
}

// newKCPTransportImpl 创建一个KCPTransportImpl示例  调用构造函数时 session 所有权移交给transport
// Socket不在这里设置 因为因为服务端 Session 与 Listener 共享UDP Socket，而客户端 Socket 配置在 DialContext 中单独完成
func newKCPTransportImpl(session *kcp.UDPSession, kcpConf KCPConfig) (transport *KCPTransportImpl, err error) {
	if session == nil {
		return nil, errors.New("network/kcp: session is nil")
	}
	if kcpConf.MTU > 0 && !session.SetMtu(kcpConf.MTU) {
		_ = session.Close()
		return nil, fmt.Errorf("network/kcp: set mtu %d failed", kcpConf.MTU)
	}
	sendWnd, recvWnd := 0, 0
	if kcpConf.SendWindow > 0 {
		sendWnd = kcpConf.SendWindow
	}
	if kcpConf.ReceiveWindow > 0 {
		recvWnd = kcpConf.ReceiveWindow
	}
	session.SetWindowSize(sendWnd, recvWnd)
	noDelay, interval, fastResend, noCongestion := -1, -1, -1, -1
	if kcpConf.NoDelay {
		noDelay = 1
	}
	if kcpConf.IntervalMs > 0 {
		interval = kcpConf.IntervalMs
	}
	if kcpConf.FastResend > 0 {
		fastResend = kcpConf.FastResend
	}
	if kcpConf.NoCongestion {
		noCongestion = 1
	}
	session.SetNoDelay(noDelay, interval, fastResend, noCongestion)
	session.SetACKNoDelay(kcpConf.AckNoDelay)
	session.SetWriteDelay(kcpConf.WriteDelay)

	return &KCPTransportImpl{session: session, kcpConf: kcpConf}, nil
}

// LocalAddr 返回本地地址
func (k *KCPTransportImpl) LocalAddr() net.Addr {
	return k.session.LocalAddr()
}

// RemoteAddr 返回远程地址
func (k *KCPTransportImpl) RemoteAddr() net.Addr {
	return k.session.RemoteAddr()
}

// Close 关闭当前Transport
func (k *KCPTransportImpl) Close() error {
	return k.session.Close()
}

// Read 读取数据
func (k *KCPTransportImpl) Read(buffer []byte) (int, error) {
	if len(buffer) <= 0 {
		return 0, nil
	}
	if k.kcpConf.ReadTimeout > 0 {
		if err := k.session.SetReadDeadline(time.Now().Add(k.kcpConf.ReadTimeout)); err != nil {
			return 0, err
		}
		defer func() {
			_ = k.session.SetReadDeadline(time.Time{})
		}()
	}
	return k.session.Read(buffer)
}

// Write 写入数据
func (k *KCPTransportImpl) Write(data []byte) (int, error) {
	if len(data) <= 0 {
		return 0, nil
	}
	if k.kcpConf.WriteTimeout > 0 {
		if err := k.session.SetWriteDeadline(time.Now().Add(k.kcpConf.WriteTimeout)); err != nil {
			return 0, err
		}
		defer func() {
			_ = k.session.SetWriteDeadline(time.Time{})
		}()
	}
	return k.session.Write(data)
}

// KCPListenerImpl 基于 kcp-go Listener 的 Listener 实现
type KCPListenerImpl struct {
	listener *kcp.Listener // kcp Listener
	kcpConf  KCPConfig     // 配置
}

// NewKCPListenerImpl 创建一个新的 KCPListenerImpl
// 当前实现不承诺 Listener.Close() 后不影响现有连接 相关的管理交由上层实现
// 如果想实现 逻辑关闭 Listener 等待现在有 Session 关闭, 再释放底层 Socket 的方式
// 需要添加相关的引用技术和资源释放逻辑, kcp 只保证单个服务端Session不影响共享Socket
func NewKCPListenerImpl(address string, kcpConf KCPConfig) (*KCPListenerImpl, error) {
	block, err := newBlockCrypt(kcpConf.Mode, append([]byte(nil), kcpConf.Key...))
	if err != nil {
		return nil, err
	}
	dataShards, parityShards := 0, 0
	if kcpConf.DataShards > 0 {
		dataShards = kcpConf.DataShards
	}
	if kcpConf.ParityShards > 0 {
		parityShards = kcpConf.ParityShards
	}
	if dataShards+parityShards > 256 {
		return nil, fmt.Errorf("network/kcp: invalid FEC shards configure, dataShards: %d, parityShards:%d", dataShards, parityShards)
	}
	ln, err := kcp.ListenWithOptions(address, block, dataShards, parityShards)
	if err != nil {
		return nil, fmt.Errorf("network/kcp: listen with address: %s options failed: %w", address, err)
	}
	// listener 创建成功后 当前构造流程已经接管其所有权
	// 设置 DSCP
	if kcpConf.DSCP > 0 {
		if err := ln.SetDSCP(kcpConf.DSCP); err != nil {
			_ = ln.Close()
			return nil, err
		}
	}
	if kcpConf.ReadBufferSize > 0 {
		if err := ln.SetReadBuffer(kcpConf.ReadBufferSize); err != nil {
			_ = ln.Close()
			return nil, err
		}
	}
	if kcpConf.WriteBufferSize > 0 {
		if err := ln.SetWriteBuffer(kcpConf.WriteBufferSize); err != nil {
			_ = ln.Close()
			return nil, err
		}
	}
	return &KCPListenerImpl{listener: ln, kcpConf: kcpConf}, nil
}

// Accept 接受一个kcp transport
func (ln *KCPListenerImpl) Accept() (Transport, error) {
	session, err := ln.listener.AcceptKCP()
	if err != nil {
		return nil, err
	}

	return newKCPTransportImpl(session, ln.kcpConf)
}

// Close 关闭Listener 底层Socket会被关闭 共享socket的Session将都失效
func (ln *KCPListenerImpl) Close() error {
	return ln.listener.Close()
}

// Addr 监听地址
func (ln *KCPListenerImpl) Addr() net.Addr {
	return ln.listener.Addr()
}

// KCPDialerImpl kcp 客户端拨号实现
type KCPDialerImpl struct {
	kcpConf KCPConfig
}

// NewKCPDialerImpl 拨号器构造函数
func NewKCPDialerImpl(kcpConf KCPConfig) *KCPDialerImpl {
	return &KCPDialerImpl{kcpConf: kcpConf}
}

// DialContext kcp 客户端拨号
func (k *KCPDialerImpl) DialContext(_ context.Context, address string) (Transport, error) {
	block, err := newBlockCrypt(k.kcpConf.Mode, append([]byte(nil), k.kcpConf.Key...))
	if err != nil {
		return nil, err
	}
	dataShards, parityShards := 0, 0
	if k.kcpConf.DataShards > 0 {
		dataShards = k.kcpConf.DataShards
	}
	if k.kcpConf.ParityShards > 0 {
		parityShards = k.kcpConf.ParityShards
	}
	if dataShards+parityShards > 256 {
		return nil, fmt.Errorf("network/kcp: invalid fec configure, dataShards: %d, parityShards:%d", dataShards, parityShards)
	}
	session, err := kcp.DialWithOptions(address, block, dataShards, parityShards)
	if err != nil {
		return nil, err
	}
	// 客户端会话独占 socket 可以直接应用socket配置
	if k.kcpConf.DSCP > 0 {
		if err := session.SetDSCP(k.kcpConf.DSCP); err != nil {
			_ = session.Close()
			return nil, err
		}
	}
	if k.kcpConf.ReadBufferSize > 0 {
		if err := session.SetReadBuffer(k.kcpConf.ReadBufferSize); err != nil {
			_ = session.Close()
			return nil, err
		}
	}
	if k.kcpConf.WriteBufferSize > 0 {
		if err := session.SetWriteBuffer(k.kcpConf.WriteBufferSize); err != nil {
			_ = session.Close()
			return nil, err
		}
	}

	return newKCPTransportImpl(session, k.kcpConf)
}

// newBlockCrypt 创建一个新的 BlockCrypt
func newBlockCrypt(mode KCPEncryption, key []byte) (kcp.BlockCrypt, error) {
	switch mode {
	case KCPEncryptionAES:
		return kcp.NewAESBlockCrypt(key)
	case KCPEncryptionNone:
		return nil, nil
	default:
		return nil, errors.New("network/kcp: unsupported encryption mode")
	}
}
