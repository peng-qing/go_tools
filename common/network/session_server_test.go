package network

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionServerShutdownOrder(t *testing.T) {
	listener := newTestListener()
	transport, _ := testPipe(t)
	opened := make(chan Connection[string], 1)
	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseCallback) }) }
	listener.onClose = func() {
		select {
		case <-transport.closed:
		default:
			t.Error("Listener 在 Conn 之前关闭")
		}
	}
	server, err := NewSessionServer[string](listener, testSharedDependencies(ConnDependencies[string]{
		Codec:   testCodec{},
		OnOpen:  func(_ context.Context, c Connection[string]) error { opened <- c; return nil },
		OnClose: func(Connection[string], error) { close(callbackEntered); <-releaseCallback },
	}), ConnConfig{})
	if err != nil {
		t.Fatal(err)
	}
	done := testServe(t, server)
	t.Cleanup(release)
	listener.incoming <- testAcceptResult{transport: transport}
	conn := testReceive(t, opened)
	if got, ok := server.Conn(conn.ID()); !ok || got != conn {
		t.Fatal("连接未登记")
	}
	// 回调期间查询和关闭服务，验证 Range 不持有连接表锁。
	server.Range(func(Connection[string]) bool { _ = server.Len(); _ = server.Close(); return false })
	testReceive(t, callbackEntered)
	if server.Len() != 1 {
		t.Fatal("OnClose 尚未完成，不应提前注销")
	}
	select {
	case <-done:
		t.Fatal("Serve 未等待 OnClose")
	default:
	}
	release()
}

func TestSessionServerCapacity(t *testing.T) {
	listener := newTestListener()
	first, _ := testPipe(t)
	second, _ := testPipe(t)
	opened := make(chan struct{}, 2)
	mgr := NewSessionManager[string](1)
	server, err := NewSessionServer[string](listener, testSharedDependencies(ConnDependencies[string]{
		Codec: testCodec{}, OnOpen: func(context.Context, Connection[string]) error { opened <- struct{}{}; return nil },
	}), ConnConfig{}, WithSessionManager[string](mgr))
	if err != nil {
		t.Fatal(err)
	}
	testServe(t, server)
	listener.incoming <- testAcceptResult{transport: first}
	testReceive(t, opened)
	if err := server.Serve(); err == nil {
		t.Fatal("应拒绝重复 Serve")
	}
	listener.incoming <- testAcceptResult{transport: second}
	testReceive(t, second.closed)
	if mgr.Len() != 1 {
		t.Fatalf("容量限制失效: %d", mgr.Len())
	}
	select {
	case <-opened:
		t.Fatal("被拒绝连接不应执行 OnOpen")
	default:
	}
}

func TestSessionServerAcceptError(t *testing.T) {
	listener := newTestListener()
	expected := errors.New("accept failed")
	listener.incoming <- testAcceptResult{err: expected}
	server, err := NewSessionServer[string](listener, testSharedDependencies(ConnDependencies[string]{Codec: testCodec{}}), ConnConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Serve(); !errors.Is(err, expected) {
		t.Fatal(err)
	}
	testReceive(t, listener.done)
	if err := server.Serve(); !errors.Is(err, net.ErrClosed) {
		t.Fatal(err)
	}
}

type testLateListener struct {
	*testListener
	entered   chan struct{}
	release   chan struct{}
	transport Transport
}

func (ln *testLateListener) Accept() (Transport, error) {
	close(ln.entered)
	<-ln.release
	return ln.transport, nil
}
func TestSessionServerLateAccept(t *testing.T) {
	transport, _ := testPipe(t)
	listener := &testLateListener{testListener: newTestListener(), entered: make(chan struct{}), release: make(chan struct{}), transport: transport}
	var factoryCalls atomic.Int32
	server, err := NewSessionServer[string](listener, func(Transport) (ConnDependencies[string], error) {
		factoryCalls.Add(1)
		return ConnDependencies[string]{Codec: testCodec{},
			OnOpen: func(context.Context, Connection[string]) error {
				t.Error("关闭后的迟到连接不应运行")
				return nil
			},
		}, nil
	}, ConnConfig{})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve() }()
	testReceive(t, listener.entered)
	_ = server.Close()
	close(listener.release)
	if err := testReceive(t, done); err != nil {
		t.Fatal(err)
	}
	testReceive(t, transport.closed)
	if server.Len() != 0 {
		t.Fatal("迟到连接被登记")
	}
	if factoryCalls.Load() != 0 {
		t.Fatal("关闭后的迟到连接不应再执行工厂")
	}
}

func TestSessionServerFactoryValidation(t *testing.T) {
	listener := newTestListener()
	if _, err := NewSessionServer[string](listener, nil, ConnConfig{}); err == nil {
		t.Fatal("应拒绝 nil 工厂")
	}
	select {
	case <-listener.done:
		t.Fatal("构造失败不应接管 Listener")
	default:
	}
	_ = listener.Close()
}

func TestSessionServerFactoryFailure(t *testing.T) {
	for _, failure := range []string{"error", "nil_codec", "panic"} {
		t.Run(failure, func(t *testing.T) {
			listener := newTestListener()
			first, _ := testPipe(t)
			second, _ := testPipe(t)
			opened := make(chan Connection[string], 1)
			calls := 0
			factory := func(Transport) (ConnDependencies[string], error) {
				calls++
				if calls == 1 {
					switch failure {
					case "error":
						return ConnDependencies[string]{}, errors.New("factory failed")
					case "nil_codec":
						return ConnDependencies[string]{}, nil
					case "panic":
						panic("factory panic")
					}
				}
				return ConnDependencies[string]{
					Codec: testCodec{},
					OnOpen: func(_ context.Context, conn Connection[string]) error {
						opened <- conn
						return nil
					},
				}, nil
			}
			server, err := NewSessionServer[string](listener, factory, ConnConfig{})
			if err != nil {
				t.Fatal(err)
			}
			testServe(t, server)
			listener.incoming <- testAcceptResult{transport: first}
			testReceive(t, first.closed)
			if server.Len() != 0 {
				t.Fatal("工厂失败的连接不应登记")
			}
			// 单连接失败不应终止监听，后续连接仍能启动。
			listener.incoming <- testAcceptResult{transport: second}
			conn := testReceive(t, opened)
			if got, ok := server.Conn(conn.ID()); !ok || got != conn {
				t.Fatal("后续连接未成功登记")
			}
		})
	}
}

func TestSessionServerCloseDuringFactory(t *testing.T) {
	listener := newTestListener()
	transport, _ := testPipe(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	server, err := NewSessionServer[string](listener, func(Transport) (ConnDependencies[string], error) {
		close(entered)
		<-release
		return ConnDependencies[string]{
			Codec: testCodec{},
			OnOpen: func(context.Context, Connection[string]) error {
				t.Error("关闭期间创建的连接不应运行")
				return nil
			},
		}, nil
	}, ConnConfig{})
	if err != nil {
		t.Fatal(err)
	}
	done := testServe(t, server)
	t.Cleanup(unblock)
	listener.incoming <- testAcceptResult{transport: transport}
	testReceive(t, entered)
	closed := make(chan error, 1)
	go func() { closed <- server.Close() }()
	if err := testReceive(t, closed); err != nil {
		t.Fatal(err)
	}
	unblock()
	testReceive(t, done)
	testReceive(t, transport.closed)
}

type testFactoryMessage struct {
	receiver *testFactoryReceiver
	connID   uint64
	packet   string
}
type testFactoryReceiver struct{ messages chan testFactoryMessage }

func (r *testFactoryReceiver) OnMessage(_ context.Context, conn Connection[string], packet string) {
	r.messages <- testFactoryMessage{receiver: r, connID: conn.ID(), packet: packet}
}

func TestSessionServerFactoryDependencies(t *testing.T) {
	for _, shared := range []bool{true, false} {
		t.Run(map[bool]string{true: "shared", false: "per_connection"}[shared], func(t *testing.T) {
			listener := newTestListener()
			messages := make(chan testFactoryMessage, 2)
			opened := make(chan Connection[string], 2)
			seen := make(chan Transport, 2)
			sharedReceiver := &testFactoryReceiver{messages: messages}
			sharedDeps := ConnDependencies[string]{
				Codec: testCodec{}, Receiver: sharedReceiver,
				OnOpen: func(_ context.Context, conn Connection[string]) error { opened <- conn; return nil },
			}
			factory := func(transport Transport) (ConnDependencies[string], error) {
				seen <- transport
				deps := sharedDeps
				if !shared {
					deps.Receiver = &testFactoryReceiver{messages: messages}
				}
				return deps, nil
			}
			server, err := NewSessionServer[string](listener, factory, ConnConfig{})
			if err != nil {
				t.Fatal(err)
			}
			testServe(t, server)
			var receivers [2]*testFactoryReceiver
			for i := 0; i < 2; i++ {
				transport, peer := testPipe(t)
				listener.incoming <- testAcceptResult{transport: transport}
				if got := testReceive(t, seen); got != transport {
					t.Fatal("工厂未收到当前连接")
				}
				conn := testReceive(t, opened)
				_ = peer.SetWriteDeadline(time.Now().Add(testTimeout))
				frame, _ := (testCodec{}).Encode("message")
				if _, err := peer.Write(frame); err != nil {
					t.Fatal(err)
				}
				msg := testReceive(t, messages)
				if msg.connID != conn.ID() || msg.packet != "message" {
					t.Fatalf("派发错误: %+v", msg)
				}
				receivers[i] = msg.receiver
			}
			if (receivers[0] == receivers[1]) != shared {
				t.Fatal("Receiver 共享方式不符合工厂返回结果")
			}
			if shared && receivers[0] != sharedReceiver {
				t.Fatal("没有使用工厂提供的共享实例")
			}
		})
	}
}
