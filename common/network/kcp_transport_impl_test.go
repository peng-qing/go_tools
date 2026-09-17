package network

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

const kcpTestWait = 3 * time.Second

// kcpAwait 限制异步操作等待时间，防止回归导致测试挂起。
func kcpAwait[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(kcpTestWait):
		t.Fatal("等待操作超时")
	}
	var zero T
	return zero
}

// kcpConfig 使用显式完整配置，不依赖尚未定义的默认值策略。
func kcpConfig() *KcpTransportConfig {
	return &KcpTransportConfig{
		ReadBufferSize: 4096, IO: &IOConfig{ReadTimeout: time.Second, WriteTimeout: time.Second},
		Protocol: KcpProtocolConfig{MTU: 1200, SendWindow: 128, ReceiveWindow: 128, NoDelay: true, Interval: 10, FastResend: 2, NoCongestion: true, AckNoDelay: true},
	}
}

func kcpListen(t *testing.T, cfg *KcpTransportConfig) *KcpListenerImpl {
	t.Helper()
	l, e := NewKcpListener("127.0.0.1:0", cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}
func kcpDial(t *testing.T, cfg *KcpTransportConfig, address string) *KcpTransportImpl {
	t.Helper()
	d, e := NewKcpDialer(cfg)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), kcpTestWait)
	defer cancel()
	tr, e := d.Dial(ctx, address)
	if e != nil {
		t.Fatal(e)
	}
	c := tr.(*KcpTransportImpl)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

type kcpAccepted struct {
	tr  Transport
	err error
}

func kcpAccept(t *testing.T, l *KcpListenerImpl) Transport {
	t.Helper()
	result := make(chan kcpAccepted, 1)
	go func() { tr, e := l.Accept(); result <- kcpAccepted{tr, e} }()
	r := kcpAwait(t, result)
	if r.err != nil {
		t.Fatal(r.err)
	}
	t.Cleanup(func() { _ = r.tr.Close() })
	return r.tr
}

// kcpPair 必须先发送首包再 Accept；KCP 不执行 TCP 式连接握手。
func kcpPair(t *testing.T, cfg *KcpTransportConfig) (*KcpTransportImpl, *KcpTransportImpl) {
	t.Helper()
	l := kcpListen(t, cfg)
	c := kcpDial(t, cfg, l.ListenerAddr().String())
	if e := c.Write([]byte("bootstrap")); e != nil {
		t.Fatal(e)
	}
	s := kcpAccept(t, l).(*KcpTransportImpl)
	got := kcpReadAll(t, s, len("bootstrap"))
	if string(got) != "bootstrap" {
		t.Fatalf("首包错误: %q", got)
	}
	return c, s
}

// kcpReadAll 聚合多次读取，不把单次 Write 错认为单次 Read。
func kcpReadAll(t *testing.T, c *KcpTransportImpl, n int) []byte {
	t.Helper()
	result := make([]byte, 0, n)
	for len(result) < n {
		b, e := c.Read()
		result = append(result, b...)
		if e != nil {
			t.Fatalf("读取长度=%d/%d, err=%v", len(result), n, e)
		}
		if len(b) == 0 {
			t.Fatal("读取没有进展")
		}
	}
	if len(result) != n {
		t.Fatalf("多读到数据: %d/%d", len(result), n)
	}
	return result
}
func kcpTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// TestKcpBidirectional 验证完整写入、双向字节流、地址、会话编号以及加密和 FEC 配置。
func TestKcpBidirectional(t *testing.T) {
	for _, mode := range []string{"明文", "AES128", "AES192", "AES256", "FEC", "AES与FEC"} {
		t.Run(mode, func(t *testing.T) {
			cfg := kcpConfig()
			switch mode {
			case "AES128", "AES与FEC":
				cfg.Encryption = KcpEncryptionConfig{Mode: KCPEncryptionAES, Key: bytes.Repeat([]byte{1}, 16)}
			case "AES192":
				cfg.Encryption = KcpEncryptionConfig{Mode: KCPEncryptionAES, Key: bytes.Repeat([]byte{2}, 24)}
			case "AES256":
				cfg.Encryption = KcpEncryptionConfig{Mode: KCPEncryptionAES, Key: bytes.Repeat([]byte{3}, 32)}
			}
			if mode == "FEC" || mode == "AES与FEC" {
				cfg.FEC = KcpFECConfig{DataShards: 2, ParityShards: 1}
			}
			c, s := kcpPair(t, cfg)
			t.Logf("actual: conv=%d/%d, local=%v, remote=%v", c.Conv(), s.Conv(), c.LocalAddr(), c.RemoteAddr())
			t.Logf("expected: addresses非空")
			if c.LocalAddr() == nil || c.RemoteAddr() == nil || s.LocalAddr() == nil || s.RemoteAddr() == nil {
				t.Fatal("地址为空")
			}
			if c.RemoteAddr().String() != s.LocalAddr().String() {
				t.Fatal("远端地址不匹配")
			}
			for _, direction := range []struct {
				from, to *KcpTransportImpl
				payload  []byte
			}{
				{c, s, bytes.Repeat([]byte("完整字节流"), 4096)}, {s, c, []byte("服务端回复")},
			} {
				e := direction.from.Write(direction.payload)
				if e != nil {
					t.Fatal(e)
				}
				got := kcpReadAll(t, direction.to, len(direction.payload))
				t.Logf("actual: length=%d, sha256=%x, write_err=%v", len(got), sha256.Sum256(got), e)
				t.Logf("expected: length=%d, sha256=%x, write_err=<nil>", len(direction.payload), sha256.Sum256(direction.payload))
				if !bytes.Equal(got, direction.payload) {
					t.Fatal("成功写入后数据不完整")
				}
			}
		})
	}
}

// TestKcpLtvSplitAndSticky 仿照旧测试覆盖拆分包头、拆分正文以及连续包，不依赖旧 Session 实现。
func TestKcpLtvSplitAndSticky(t *testing.T) {
	for _, size := range []int{3, 257, 4096} {
		t.Run(fmt.Sprintf("读取缓冲%d", size), func(t *testing.T) {
			cfg := kcpConfig()
			cfg.ReadBufferSize = size
			c, s := kcpPair(t, cfg)
			codec := NewLtvCodec(1<<20, false)
			wants := []*LtvPacket{NewLtvPacket(1, []byte("头部")), NewLtvPacket(2, bytes.Repeat([]byte("payload"), 1024)), NewLtvPacket(3, nil)}
			var wire []byte
			for _, p := range wants {
				b, e := codec.Encode(p)
				if e != nil {
					t.Fatal(e)
				}
				wire = append(wire, b...)
			}
			if e := c.Write(wire[:3]); e != nil {
				t.Fatal(e)
			}
			prefix := kcpReadAll(t, s, 3)
			p, n, e := codec.Decode(prefix)
			t.Logf("actual: partial_packet=%v, consumed=%d, err=%v", p, n, e)
			t.Log("expected: partial_packet=<nil>, consumed=0, err=<nil>")
			if p != nil || n != 0 || e != nil {
				t.Fatal("半包误报")
			}
			if e := c.Write(wire[3:]); e != nil {
				t.Fatal(e)
			}
			pending := prefix
			for _, want := range wants {
				for {
					got, n, e := codec.Decode(pending)
					if e != nil {
						t.Fatal(e)
					}
					if got != nil {
						t.Logf("actual: type=%d, length=%d, sha256=%x", got.Type, len(got.Payload), sha256.Sum256(got.Payload))
						t.Logf("expected: type=%d, length=%d, sha256=%x", want.Type, len(want.Payload), sha256.Sum256(want.Payload))
						if got.Type != want.Type || !bytes.Equal(got.Payload, want.Payload) {
							t.Fatal("包内容或顺序错误")
						}
						pending = pending[n:]
						break
					}
					b, e := s.Read()
					if e != nil {
						t.Fatal(e)
					}
					pending = append(pending, b...)
				}
			}
			if len(pending) != 0 {
				t.Fatal("消费后仍有剩余字节")
			}
		})
	}
}

// TestKcpReadTimeoutRecovery 验证读超时可以恢复，后续数据独立持有且未被再次读取覆盖。
func TestKcpReadTimeoutRecovery(t *testing.T) {
	cfg := kcpConfig()
	cfg.IO.ReadTimeout = 60 * time.Millisecond
	c, s := kcpPair(t, cfg)
	b, e := c.Read()
	t.Logf("actual: data=%q, err=%v, timeout=%v", b, e, kcpTimeout(e))
	t.Log("expected: data为空, timeout=true")
	if len(b) != 0 || !kcpTimeout(e) {
		t.Fatal("未返回读取超时")
	}
	if e := s.Write([]byte("first")); e != nil {
		t.Fatal(e)
	}
	first := kcpReadAll(t, c, 5)
	if e := s.Write([]byte("other")); e != nil {
		t.Fatal(e)
	}
	second := kcpReadAll(t, c, 5)
	t.Logf("actual: first=%q, second=%q", first, second)
	t.Log("expected: first=\"first\", second=\"other\"")
	if string(first) != "first" || string(second) != "other" {
		t.Fatal("超时恢复或内存所有权错误")
	}
}

// kcpBlackhole 使用真实 UDP 接收地址但不应答，使发送窗口稳定填满。
func kcpBlackhole(t *testing.T, cfg *KcpTransportConfig) *KcpTransportImpl {
	t.Helper()
	sink, e := net.ListenPacket("udp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = sink.Close() })
	return kcpDial(t, cfg, sink.LocalAddr().String())
}

// TestKcpWriteBackpressure 验证成功仅表示入发送队列；无 ACK 时后续写入超时或被 Close 解除。
func TestKcpWriteBackpressure(t *testing.T) {
	for _, closeIt := range []bool{false, true} {
		name := "写超时"
		if closeIt {
			name = "关闭解除阻塞写"
		}
		t.Run(name, func(t *testing.T) {
			cfg := kcpConfig()
			cfg.Protocol.SendWindow = 1
			if closeIt {
				cfg.IO.WriteTimeout = 0
			} else {
				cfg.IO.WriteTimeout = 60 * time.Millisecond
			}
			c := kcpBlackhole(t, cfg)
			e := c.Write(bytes.Repeat([]byte("x"), 2400))
			t.Logf("actual: queued_write_err=%v", e)
			t.Log("expected: queued_write_err=<nil>")
			if e != nil {
				t.Fatal(e)
			}
			result := make(chan error, 1)
			go func() { result <- c.Write([]byte("blocked")) }()
			if closeIt {
				select {
				case e := <-result:
					t.Fatalf("窗口满仍提前返回: %v", e)
				case <-time.After(20 * time.Millisecond):
				}
				if e := c.Close(); e != nil {
					t.Fatal(e)
				}
			}
			e = kcpAwait(t, result)
			t.Logf("actual: err=%v, timeout=%v, closed_pipe=%v", e, kcpTimeout(e), errors.Is(e, io.ErrClosedPipe))
			if closeIt {
				t.Log("expected: closed_pipe=true")
				if !errors.Is(e, io.ErrClosedPipe) {
					t.Fatal("关闭未解除写入")
				}
			} else {
				t.Log("expected: timeout=true")
				if !kcpTimeout(e) {
					t.Fatal("未返回写超时")
				}
			}
		})
	}
}

// TestKcpConcurrentClose 验证并发及重复关闭幂等，阻塞读取解除，关闭后非空写入失败。
func TestKcpConcurrentClose(t *testing.T) {
	cfg := kcpConfig()
	cfg.IO = &IOConfig{}
	c, _ := kcpPair(t, cfg)
	reads := make(chan error, 1)
	go func() { _, e := c.Read(); reads <- e }()
	select {
	case e := <-reads:
		t.Fatalf("读取提前返回: %v", e)
	case <-time.After(20 * time.Millisecond):
	}
	closed := make(chan error, 12)
	for i := 0; i < 12; i++ {
		go func() { closed <- c.Close() }()
	}
	for i := 0; i < 12; i++ {
		e := kcpAwait(t, closed)
		t.Logf("actual: close[%d]=%v", i, e)
		t.Logf("expected: close[%d]=<nil>", i)
		if e != nil {
			t.Fatal(e)
		}
	}
	readErr := kcpAwait(t, reads)
	writeErr := c.Write([]byte("closed"))
	t.Logf("actual: read_err=%v, write_err=%v", readErr, writeErr)
	t.Log("expected: read_err和write_err均包装io.ErrClosedPipe")
	if !errors.Is(readErr, io.ErrClosedPipe) || !errors.Is(writeErr, io.ErrClosedPipe) {
		t.Fatal("关闭结果错误")
	}
}

// TestKcpPeerCloseUsesTimeout 验证完整尾数据已收到后，对端关闭不会被误判为 TCP EOF。
func TestKcpPeerCloseUsesTimeout(t *testing.T) {
	cfg := kcpConfig()
	cfg.IO.ReadTimeout = 60 * time.Millisecond
	c, s := kcpPair(t, cfg)
	if e := s.Write([]byte("last")); e != nil {
		t.Fatal(e)
	}
	b := kcpReadAll(t, c, 4)
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	_, e := c.Read()
	t.Logf("actual: final_data=%q, err=%v, timeout=%v, eof=%v", b, e, kcpTimeout(e), errors.Is(e, io.EOF))
	t.Log("expected: final_data=\"last\", timeout=true, eof=false")
	if string(b) != "last" || !kcpTimeout(e) || errors.Is(e, io.EOF) {
		t.Fatal("KCP远端关闭语义不符")
	}
}

// TestKcpListenerCloseBroadcast 验证关闭解除多个 Accept，且后续调用不阻塞。
func TestKcpListenerCloseBroadcast(t *testing.T) {
	l := kcpListen(t, kcpConfig())
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			tr, e := l.Accept()
			if tr != nil {
				_ = tr.Close()
			}
			results <- e
		}()
	}
	if e := l.Close(); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 8; i++ {
		e := kcpAwait(t, results)
		t.Logf("actual: accept[%d]=%v", i, e)
		t.Logf("expected: accept[%d]包装io.ErrClosedPipe或net.ErrClosed", i)
		if !(errors.Is(e, io.ErrClosedPipe) || errors.Is(e, net.ErrClosed)) {
			t.Fatal(e)
		}
	}
	for i := 0; i < 3; i++ {
		e := l.Close()
		if e != nil {
			t.Fatal(e)
		}
		go func() { _, e := l.Accept(); results <- e }()
		e = kcpAwait(t, results)
		t.Logf("actual: subsequent_accept=%v", e)
		t.Log("expected: subsequent_accept包装io.ErrClosedPipe或net.ErrClosed")
		if !(errors.Is(e, io.ErrClosedPipe) || errors.Is(e, net.ErrClosed)) {
			t.Fatal(e)
		}
	}
}

// TestKcpDialInvalidInputs 验证上下文和地址解析错误，无需访问外部网络。
func TestKcpDialInvalidInputs(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	for _, tc := range []struct {
		name, address string
		ctx           context.Context
		want          error
	}{
		{"取消", "127.0.0.1:1", canceled, context.Canceled}, {"过期", "127.0.0.1:1", expired, context.DeadlineExceeded},
		{"缺少端口", "127.0.0.1", context.Background(), nil}, {"非数字端口", "127.0.0.1:abc", context.Background(), nil},
		{"负端口", "127.0.0.1:-1", context.Background(), nil}, {"超大端口", "127.0.0.1:65536", context.Background(), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, e := NewKcpDialer(kcpConfig())
			if e != nil {
				t.Fatal(e)
			}
			tr, e := d.Dial(tc.ctx, tc.address)
			if tr != nil {
				defer tr.Close()
			}
			t.Logf("actual: transport=%v, err=%v", tr, e)
			t.Logf("expected: transport=<nil>, err非空, sentinel=%v", tc.want)
			if tr != nil || e == nil || (tc.want != nil && !errors.Is(e, tc.want)) {
				t.Fatal("拨号未拒绝非法输入")
			}
		})
	}
}

// TestKcpEncryptionRejected 验证实际创建会话时拒绝错误 AES 密钥和未知加密模式。
func TestKcpEncryptionRejected(t *testing.T) {
	for _, encryption := range []KcpEncryptionConfig{{Mode: KCPEncryptionAES, Key: []byte("short")}, {Mode: KCPEncryption("unknown")}} {
		t.Run(string(encryption.Mode), func(t *testing.T) {
			cfg := kcpConfig()
			cfg.Encryption = encryption
			l, le := NewKcpListener("127.0.0.1:0", cfg)
			if l != nil {
				defer l.Close()
			}
			d, e := NewKcpDialer(cfg)
			if e != nil {
				t.Fatal(e)
			}
			tr, de := d.Dial(context.Background(), "127.0.0.1:1")
			if tr != nil {
				defer tr.Close()
			}
			t.Logf("actual: listener=%v, listener_err=%v, transport=%v, dial_err=%v", l, le, tr, de)
			t.Log("expected: listener=<nil>, transport=<nil>, 两个错误均非空")
			if l != nil || tr != nil || le == nil || de == nil {
				t.Fatal("非法加密未被拒绝")
			}
		})
	}
}

// TestKcpInvalidMTU 验证第三方拒绝的 MTU 在拨号和接收路径都返回错误。
func TestKcpInvalidMTU(t *testing.T) {
	cfg := kcpConfig()
	cfg.Protocol.MTU = 1
	d, e := NewKcpDialer(cfg)
	if e != nil {
		t.Fatal(e)
	}
	tr, e := d.Dial(context.Background(), "127.0.0.1:1")
	if tr != nil {
		defer tr.Close()
	}
	t.Logf("actual: dial_transport=%v, err=%v", tr, e)
	t.Log("expected: dial_transport=<nil>, err非空")
	if tr != nil || e == nil {
		t.Fatal("拨号未拒绝MTU")
	}
	l := kcpListen(t, cfg)
	c := kcpDial(t, kcpConfig(), l.ListenerAddr().String())
	if e := c.Write([]byte("trigger")); e != nil {
		t.Fatal(e)
	}
	result := make(chan kcpAccepted, 1)
	go func() { tr, e := l.Accept(); result <- kcpAccepted{tr, e} }()
	r := kcpAwait(t, result)
	if r.tr != nil {
		defer r.tr.Close()
	}
	t.Logf("actual: accept_transport=%v, err=%v", r.tr, r.err)
	t.Log("expected: accept_transport=<nil>, err非空")
	if r.tr != nil || r.err == nil {
		t.Fatal("接收未拒绝MTU")
	}
}

// TestKcpListenerSocketBuffers 验证监听器设置合法 UDP 缓冲区后仍可正常交付连接。
func TestKcpListenerSocketBuffers(t *testing.T) {
	for _, read := range []bool{true, false} {
		name := "接收缓冲"
		if !read {
			name = "发送缓冲"
		}
		t.Run(name, func(t *testing.T) {
			cfg := kcpConfig()
			if read {
				cfg.Socket.SocketReadBuffer = 64 << 10
			} else {
				cfg.Socket.SocketWriteBuffer = 64 << 10
			}
			l := kcpListen(t, cfg)
			c := kcpDial(t, kcpConfig(), l.ListenerAddr().String())
			if e := c.Write([]byte("socket-options")); e != nil {
				t.Fatal(e)
			}
			result := make(chan kcpAccepted, 1)
			go func() { tr, e := l.Accept(); result <- kcpAccepted{tr, e} }()
			r := kcpAwait(t, result)
			if r.tr != nil {
				defer r.tr.Close()
			}
			t.Logf("actual: accepted=%v, err=%v", r.tr != nil, r.err)
			t.Log("expected: accepted=true, err=<nil>")
			if r.tr == nil || r.err != nil {
				t.Fatalf("合法socket配置导致Accept失败: %v", r.err)
			}
		})
	}
}

// TestKcpDialWithoutHandshake 验证空上下文可用，拨号不等待对端握手，首包前 Accept 保持等待。
func TestKcpDialWithoutHandshake(t *testing.T) {
	cfg := kcpConfig()
	l := kcpListen(t, cfg)
	d, e := NewKcpDialer(cfg)
	if e != nil {
		t.Fatal(e)
	}
	tr, e := d.Dial(context.Background(), l.ListenerAddr().String())
	t.Logf("actual: dial_success=%v, err=%v", tr != nil, e)
	t.Log("expected: dial_success=true, err=<nil>")
	if e != nil || tr == nil {
		t.Fatal("拨号未成功")
	}
	defer tr.Close()
	results := make(chan kcpAccepted, 1)
	go func() { c, e := l.Accept(); results <- kcpAccepted{c, e} }()
	select {
	case r := <-results:
		if r.tr != nil {
			_ = r.tr.Close()
		}
		t.Fatalf("首包之前Accept提前返回: %v", r.err)
	case <-time.After(20 * time.Millisecond):
		t.Log("actual: accept_pending=true")
		t.Log("expected: accept_pending=true")
	}
	if e := tr.Write([]byte("first")); e != nil {
		t.Fatal(e)
	}
	r := kcpAwait(t, results)
	if r.tr != nil {
		defer r.tr.Close()
	}
	t.Logf("actual: accepted=%v, err=%v", r.tr != nil, r.err)
	t.Log("expected: accepted=true, err=<nil>")
	if r.tr == nil || r.err != nil {
		t.Fatal("首包后没有接收连接")
	}
}

// TestKcpDialSocketBuffers 验证独立拨号 socket 的缓冲区设置可以正常生效并通信。
func TestKcpDialSocketBuffers(t *testing.T) {
	l := kcpListen(t, kcpConfig())
	cfg := kcpConfig()
	cfg.Socket.SocketReadBuffer = 64 << 10
	cfg.Socket.SocketWriteBuffer = 64 << 10
	c := kcpDial(t, cfg, l.ListenerAddr().String())
	if e := c.Write([]byte("buffers")); e != nil {
		t.Fatal(e)
	}
	s := kcpAccept(t, l).(*KcpTransportImpl)
	data := kcpReadAll(t, s, 7)
	t.Logf("actual: received=%q", data)
	t.Log("expected: received=\"buffers\"")
	if string(data) != "buffers" {
		t.Fatal("设置拨号socket缓冲区后通信失败")
	}
}

// TestKcpListenerAddressErrors 验证非法地址及重复占用端口返回明确错误。
func TestKcpListenerAddressErrors(t *testing.T) {
	used := kcpListen(t, kcpConfig())
	for _, address := range []string{"invalid-address", used.ListenerAddr().String()} {
		t.Run(address, func(t *testing.T) {
			l, e := NewKcpListener(address, kcpConfig())
			if l != nil {
				defer l.Close()
			}
			t.Logf("actual: listener=%v, err=%v", l, e)
			t.Log("expected: listener=<nil>, err非空")
			if l != nil || e == nil {
				t.Fatal("非法或冲突地址未被拒绝")
			}
		})
	}
}

// TestKcpSiblingSessionIsolation 验证关闭一个会话不会关闭共享监听器及其他会话。
func TestKcpSiblingSessionIsolation(t *testing.T) {
	cfg := kcpConfig()
	l := kcpListen(t, cfg)
	c1 := kcpDial(t, cfg, l.ListenerAddr().String())
	if e := c1.Write([]byte("one")); e != nil {
		t.Fatal(e)
	}
	s1 := kcpAccept(t, l).(*KcpTransportImpl)
	_ = kcpReadAll(t, s1, 3)
	c2 := kcpDial(t, cfg, l.ListenerAddr().String())
	if e := c2.Write([]byte("two")); e != nil {
		t.Fatal(e)
	}
	s2 := kcpAccept(t, l).(*KcpTransportImpl)
	_ = kcpReadAll(t, s2, 3)
	if e := s1.Close(); e != nil {
		t.Fatal(e)
	}
	if e := c1.Close(); e != nil {
		t.Fatal(e)
	}
	if e := c2.Write([]byte("still-alive")); e != nil {
		t.Fatal(e)
	}
	got := kcpReadAll(t, s2, len("still-alive"))
	t.Logf("actual: surviving_session_data=%q", got)
	t.Log("expected: surviving_session_data=\"still-alive\"")
	if string(got) != "still-alive" {
		t.Fatal("关闭同级会话影响其他连接")
	}
}

// TestKcpReadNoProgress 验证零长读取缓冲有明确无进展错误，不伪报成功。
func TestKcpReadNoProgress(t *testing.T) {
	c, s := kcpPair(t, kcpConfig())
	// 创建配置副本，避免修改连接对共同持有的配置。
	cfg := *c.config
	cfg.ReadBufferSize = 0
	c.config = &cfg
	if e := s.Write([]byte("pending")); e != nil {
		t.Fatal(e)
	}
	data, e := c.Read()
	t.Logf("actual: data=%q, err=%v", data, e)
	t.Log("expected: data为空, err=io.ErrNoProgress")
	if len(data) != 0 || !errors.Is(e, io.ErrNoProgress) {
		t.Fatal("零缓冲错误结果不符")
	}
}
