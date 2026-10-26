package network

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestConnStreamFrames(t *testing.T) {
	transport, peer := testPipe(t)
	packets := make(chan string, 3)
	conn := testNewConn(t, transport, ConnDependencies[string]{Codec: testCodec{},
		Receiver: testReceiver(func(_ context.Context, _ Connection[string], p string) { packets <- p }),
	}, ConnConfig{ReadBufferSize: 16})
	done := testRunConn(t, conn)
	// 第一帧分段写入，后两帧一次写入，覆盖半包和粘包。
	a, _ := (testCodec{}).Encode("hello")
	b, _ := (testCodec{}).Encode(strings.Repeat("x", 80))
	c, _ := (testCodec{}).Encode("end")
	writeDone := make(chan error, 1)
	go func() {
		for _, part := range [][]byte{a[:1], a[1:3], a[3:], append(b, c...)} {
			if _, err := peer.Write(part); err != nil {
				writeDone <- err
				return
			}
		}
		writeDone <- nil
	}()
	for _, want := range []string{"hello", strings.Repeat("x", 80), "end"} {
		if got := testReceive(t, packets); got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	}
	if err := testReceive(t, writeDone); err != nil {
		t.Fatal(err)
	}
	_ = peer.Close()
	testReceive(t, done)
	if !errors.Is(conn.Err(), io.EOF) {
		t.Fatalf("终止原因: %v", conn.Err())
	}
	want := TrafficSnapshot{ReadMessages: 3, ReadBytes: 91}
	if got := conn.Statistics(); got != want {
		t.Fatalf("半包及粘包统计: got %+v want %+v", got, want)
	}
}

func TestConnTruncatedFrame(t *testing.T) {
	transport, peer := testPipe(t)
	conn := testNewConn(t, transport, ConnDependencies[string]{Codec: testCodec{}}, ConnConfig{})
	done := testRunConn(t, conn)
	if _, err := peer.Write([]byte{5, 'a'}); err != nil {
		t.Fatal(err)
	}
	_ = peer.Close()
	testReceive(t, done)
	if !errors.Is(conn.Err(), io.ErrUnexpectedEOF) {
		t.Fatalf("终止原因: %v", conn.Err())
	}
}

func TestConnSendQueue(t *testing.T) {
	transport, _ := testPipe(t)
	conn := testNewConn(t, transport, ConnDependencies[string]{Codec: testCodec{}}, ConnConfig{SendQueueSize: 1})
	if err := conn.TrySend("first"); err != nil {
		t.Fatal(err)
	}
	if err := conn.TrySend("second"); !errors.Is(err, ErrSendQueueFull) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := conn.Send(ctx, "canceled"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	started := make(chan struct{})
	sent := make(chan error, 1)
	go func() { close(started); sent <- conn.Send(context.Background(), "blocked") }()
	testReceive(t, started)
	_ = conn.Close()
	if err := testReceive(t, sent); err == nil {
		t.Fatal("关闭后阻塞发送应退出")
	}
	if err := conn.TrySend("closed"); err == nil {
		t.Fatal("已关闭连接不应入队")
	}
}

type testPanicCodec struct{ testCodec }

func (testPanicCodec) Decode(io.Reader) (string, int, error) { panic("decode failed") }

type testWriteTransport struct {
	Transport
	panicWrite bool
}

func (w testWriteTransport) Write(p []byte) (int, error) {
	if w.panicWrite {
		panic("write failed")
	}
	return len(p) - 1, nil
}

type testPanicHeartbeat struct{ testHeartbeat }

func (testPanicHeartbeat) Ping(uint64) string { panic("ping failed") }

func TestConnPanicTerminates(t *testing.T) {
	for _, kind := range []string{"read", "write", "heartbeat", "OnOpen", "OnMessage", "OnClose"} {
		t.Run(kind, func(t *testing.T) {
			transport, peer := testPipe(t)
			var rwc Transport = transport
			deps := ConnDependencies[string]{Codec: testCodec{}}
			conf := ConnConfig{}
			closed := make(chan struct{})
			deps.OnClose = func(Connection[string], error) { close(closed) }
			switch kind {
			case "read":
				deps.Codec = testPanicCodec{}
			case "write":
				rwc = testWriteTransport{Transport: transport, panicWrite: true}
				deps.OnOpen = func(ctx context.Context, c Connection[string]) error { return c.Send(ctx, "hello") }
			case "heartbeat":
				deps.Heartbeator = testPanicHeartbeat{}
				conf.HeartbeatInterval = time.Millisecond
				conf.HeartbeatTimeout = time.Hour
			case "OnOpen":
				deps.OnOpen = func(context.Context, Connection[string]) error { panic("open failed") }
			case "OnMessage":
				deps.Receiver = testReceiver(func(context.Context, Connection[string], string) { panic("message failed") })
			case "OnClose":
				deps.OnOpen = func(context.Context, Connection[string]) error { return io.EOF }
				deps.OnClose = func(Connection[string], error) { close(closed); panic("close failed") }
			}
			conn := testNewConn(t, rwc, deps, conf)
			done := testRunConn(t, conn)
			if kind == "OnMessage" {
				_ = peer.SetWriteDeadline(time.Now().Add(testTimeout))
				if _, err := peer.Write([]byte{1, 'x'}); err != nil {
					t.Fatal(err)
				}
			}
			testReceive(t, done)
			testReceive(t, closed)
			testReceive(t, transport.closed)
			if kind == "OnClose" {
				if !errors.Is(conn.Err(), io.EOF) {
					t.Fatalf("OnClose 不应覆盖原因: %v", conn.Err())
				}
			} else if conn.Err() == nil || !strings.Contains(conn.Err().Error(), kind) {
				t.Fatalf("终止原因未记录 panic: %v", conn.Err())
			}
		})
	}
}

func TestConnShortWrite(t *testing.T) {
	transport, _ := testPipe(t)
	conn := testNewConn(t, testWriteTransport{Transport: transport}, ConnDependencies[string]{
		Codec: testCodec{}, OnOpen: func(ctx context.Context, c Connection[string]) error { return c.Send(ctx, "hello") },
	}, ConnConfig{})
	done := testRunConn(t, conn)
	testReceive(t, done)
	if !errors.Is(conn.Err(), io.ErrShortWrite) {
		t.Fatal(conn.Err())
	}
}

func TestConnHeartbeat(t *testing.T) {
	t.Run("response_only", func(t *testing.T) {
		transport, peer := testPipe(t)
		business := make(chan string, 1)
		conn := testNewConn(t, transport, ConnDependencies[string]{Codec: testCodec{}, Heartbeator: testHeartbeat{},
			Receiver: testReceiver(func(_ context.Context, _ Connection[string], p string) { business <- p }),
		}, ConnConfig{})
		testRunConn(t, conn)
		_ = peer.SetDeadline(time.Now().Add(testTimeout))
		heartbeat := testHeartbeat{}
		for _, seq := range []uint64{0, 42, 1<<64 - 1} {
			frame, _ := (testCodec{}).Encode(heartbeat.Ping(seq))
			if _, err := peer.Write(frame); err != nil {
				t.Fatal(err)
			}
			packet, _, err := (testCodec{}).Decode(peer)
			if err != nil {
				t.Fatal(err)
			}
			want := "pong:" + strconv.FormatUint(seq, 10)
			if packet != want {
				t.Fatalf("Pong 未回传原始序号: got %q want %q", packet, want)
			}
		}
		// 先发送 Pong，再发送业务帧，确认两种心跳都不会交给 Receiver。
		frame, _ := (testCodec{}).Encode(heartbeat.Pong(42))
		if _, err := peer.Write(frame); err != nil {
			t.Fatal(err)
		}
		frame, _ = (testCodec{}).Encode("business")
		if _, err := peer.Write(frame); err != nil {
			t.Fatal(err)
		}
		if packet := testReceive(t, business); packet != "business" {
			t.Fatalf("心跳不应交给业务: %q", packet)
		}
	})
	t.Run("ping_sequence", func(t *testing.T) {
		transport, peer := testPipe(t)
		conn := testNewConn(t, transport, ConnDependencies[string]{
			Codec: testCodec{}, Heartbeator: testHeartbeat{},
		}, ConnConfig{HeartbeatInterval: 10 * time.Millisecond, HeartbeatTimeout: time.Hour})
		done := testRunConn(t, conn)
		_ = peer.SetReadDeadline(time.Now().Add(testTimeout))
		for seq := uint64(1); seq <= 3; seq++ {
			packet, _, err := (testCodec{}).Decode(peer)
			if err != nil {
				t.Fatal(err)
			}
			want := "ping:" + strconv.FormatUint(seq, 10)
			if packet != want {
				t.Fatalf("主动 Ping 序号错误: got %q want %q", packet, want)
			}
		}
		_ = conn.Close()
		testReceive(t, done)
		want := TrafficSnapshot{WriteMessages: 3, WriteBytes: 21}
		if got := conn.Statistics(); got != want {
			t.Fatalf("主动 Ping 统计: got %+v want %+v", got, want)
		}
	})
	t.Run("timeout_with_blocked_writer", func(t *testing.T) {
		transport, _ := testPipe(t)
		conn := testNewConn(t, transport, ConnDependencies[string]{Codec: testCodec{}, Heartbeator: testHeartbeat{}},
			ConnConfig{SendQueueSize: 1, HeartbeatInterval: 10 * time.Millisecond, HeartbeatTimeout: 50 * time.Millisecond})
		done := testRunConn(t, conn)
		testReceive(t, done)
		if !errors.Is(conn.Err(), ErrHeartbeatTimeout) {
			t.Fatal(conn.Err())
		}
	})
}
