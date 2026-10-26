package network

import (
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testTimeout = 5 * time.Second

// testCodec 使用一个字节长度头，便于验证半包、粘包和截断帧。
type testCodec struct{}

func (testCodec) Encode(packet string) ([]byte, error) {
	return append([]byte{byte(len(packet))}, []byte(packet)...), nil
}
func (testCodec) Decode(reader io.Reader) (string, error) {
	var head [1]byte
	if _, err := io.ReadFull(reader, head[:]); err != nil {
		return "", err
	}
	data := make([]byte, int(head[0]))
	_, err := io.ReadFull(reader, data)
	return string(data), err
}

type testReceiver func(context.Context, Connection[string], string)

func (fn testReceiver) OnMessage(ctx context.Context, conn Connection[string], packet string) {
	fn(ctx, conn, packet)
}

type testDialer func(context.Context, string) (Transport, error)

func (fn testDialer) DialContext(ctx context.Context, address string) (Transport, error) {
	return fn(ctx, address)
}

type testHeartbeat struct{}

func (testHeartbeat) Ping(seq uint64) string { return "ping:" + strconv.FormatUint(seq, 10) }
func (testHeartbeat) Pong(seq uint64) string { return "pong:" + strconv.FormatUint(seq, 10) }
func (testHeartbeat) Inspect(packet string) (HeartbeatKind, uint64, bool) {
	name, value, ok := strings.Cut(packet, ":")
	if !ok {
		return HeartbeatKind_Invalid, 0, false
	}
	var kind HeartbeatKind
	switch name {
	case "ping":
		kind = HeartbeatKind_Ping
	case "pong":
		kind = HeartbeatKind_Pong
	default:
		return HeartbeatKind_Invalid, 0, false
	}
	seq, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return HeartbeatKind_Invalid, 0, false
	}
	return kind, seq, true
}

// testTrackedTransport 在实际关闭后发布信号，用于检查资源释放顺序。
type testTrackedTransport struct {
	net.Conn
	once     sync.Once
	closed   chan struct{}
	closeErr error
}

func (c *testTrackedTransport) Close() error {
	c.once.Do(func() { c.closeErr = c.Conn.Close(); close(c.closed) })
	return c.closeErr
}
func testPipe(t *testing.T) (*testTrackedTransport, net.Conn) {
	t.Helper()
	a, b := net.Pipe()
	conn := &testTrackedTransport{Conn: a, closed: make(chan struct{})}
	t.Cleanup(func() { _ = conn.Close(); _ = b.Close() })
	return conn, b
}
func testReceive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(testTimeout):
		t.Fatal("等待操作完成超时")
		var zero T
		return zero
	}
}
func testRunConn(t *testing.T, conn *Conn[string]) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() { conn.Run(); close(done) }()
	t.Cleanup(func() { _ = conn.Close(); testReceive(t, done) })
	return done
}
func testNewConn(t *testing.T, transport Transport, deps ConnDependencies[string], conf ConnConfig) *Conn[string] {
	t.Helper()
	conn, err := newConn(transport, deps, conf)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

type testSession struct {
	Connection[string]
	id       uint64
	closes   atomic.Int32
	closeErr error
}

func (s *testSession) ID() uint64   { return s.id }
func (s *testSession) Close() error { s.closes.Add(1); return s.closeErr }

type testAcceptResult struct {
	transport Transport
	err       error
}
type testListener struct {
	incoming chan testAcceptResult
	done     chan struct{}
	once     sync.Once
	onClose  func()
}

func newTestListener() *testListener {
	return &testListener{incoming: make(chan testAcceptResult, 4), done: make(chan struct{})}
}
func (ln *testListener) Accept() (Transport, error) {
	select {
	case result := <-ln.incoming:
		return result.transport, result.err
	case <-ln.done:
		return nil, net.ErrClosed
	}
}
func (ln *testListener) Addr() net.Addr { return &net.TCPAddr{} }
func (ln *testListener) Close() error {
	ln.once.Do(func() {
		if ln.onClose != nil {
			ln.onClose()
		}
		close(ln.done)
	})
	return nil
}

// testSharedDependencies 将共享依赖适配为工厂，不创建新的业务实例。
func testSharedDependencies(deps ConnDependencies[string]) ConnDependenciesFactory[string] {
	return func(Transport) (ConnDependencies[string], error) { return deps, nil }
}

// testServe 发布可重复等待的结果，清理阶段也会验证 Serve 的返回值。
func testServe(t *testing.T, server *SessionServer[string]) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	var serveErr error
	go func() {
		serveErr = server.Serve()
		close(done)
	}()
	t.Cleanup(func() {
		_ = server.Close()
		testReceive(t, done)
		if serveErr != nil {
			t.Errorf("Serve: %v", serveErr)
		}
		if server.Len() != 0 {
			t.Errorf("Serve 结束后仍有 %d 条连接", server.Len())
		}
	})
	return done
}
