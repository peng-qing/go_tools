package network

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

func TestSessionClientSuccessAndCallbackCompletion(t *testing.T) {
	transport, peer := testPipe(t)
	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseCallback) }) }
	client, err := NewSessionClient[string](
		testDialer(func(context.Context, string) (Transport, error) { return transport, nil }),
		ConnDependencies[string]{Codec: testCodec{}, OnClose: func(Connection[string], error) {
			close(callbackEntered)
			<-releaseCallback
		}}, ConnConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	// Cleanup 顺序保证任何断言失败都不会把 OnClose 留在阻塞状态。
	t.Cleanup(release)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := client.DialContext(ctx, "unused")
	if err != nil || conn == nil {
		t.Fatalf("拨号: conn=%v err=%v", conn, err)
	}
	cancel()
	// 拨号 context 取消后，连接仍然允许发送。
	sent := make(chan error, 1)
	go func() { sent <- conn.Send(context.Background(), "hello") }()
	_ = peer.SetReadDeadline(time.Now().Add(testTimeout))
	packet, _, err := (testCodec{}).Decode(peer)
	if err != nil || packet != "hello" {
		t.Fatalf("读取: %q %v", packet, err)
	}
	if err := testReceive(t, sent); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DialContext(nil, "unused"); err == nil {
		t.Fatal("应拒绝重复拨号")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	testReceive(t, callbackEntered)
	select {
	case <-client.done:
		t.Fatal("OnClose 未结束时已发布完成")
	default:
	}
	release()
	if err := client.Wait(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Wait: %v", err)
	}
	if err := client.Wait(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("重复 Wait: %v", err)
	}
}

func TestSessionClientCloseBeforeDial(t *testing.T) {
	client, err := NewSessionClient[string](testDialer(func(context.Context, string) (Transport, error) {
		t.Error("关闭后不应拨号")
		return nil, nil
	}), ConnDependencies[string]{Codec: testCodec{}}, ConnConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Wait(); err == nil {
		t.Fatal("未启动时 Wait 应返回错误")
	}
	_ = client.Close()
	if _, err := client.DialContext(nil, "unused"); !errors.Is(err, net.ErrClosed) {
		t.Fatal(err)
	}
	if err := client.Wait(); !errors.Is(err, net.ErrClosed) {
		t.Fatal(err)
	}
}

func TestSessionClientDialFailure(t *testing.T) {
	for _, invalidTransport := range []bool{false, true} {
		t.Run(map[bool]string{false: "dial_error", true: "nil_transport"}[invalidTransport], func(t *testing.T) {
			transport, _ := testPipe(t)
			expected := errors.New("dial failed")
			client, _ := NewSessionClient[string](testDialer(func(context.Context, string) (Transport, error) {
				if invalidTransport {
					return nil, nil
				}
				return transport, expected
			}), ConnDependencies[string]{Codec: testCodec{}}, ConnConfig{})
			defer client.Close()
			_, err := client.DialContext(nil, "unused")
			if err == nil {
				t.Fatal("应返回错误")
			}
			if !invalidTransport {
				if !errors.Is(err, expected) {
					t.Fatal(err)
				}
				testReceive(t, transport.closed)
			}
			if client.Wait() != err {
				t.Fatal("Wait 未保留错误")
			}
			if client.dialCancel != nil {
				t.Fatal("未清理取消函数")
			}
			if _, err := client.DialContext(nil, "unused"); err == nil {
				t.Fatal("失败后不应再次拨号")
			}
		})
	}
}

func TestSessionClientLateDialResult(t *testing.T) {
	for _, byClose := range []bool{false, true} {
		t.Run(map[bool]string{false: "context_cancel", true: "client_close"}[byClose], func(t *testing.T) {
			transport, _ := testPipe(t)
			entered, release := make(chan struct{}), make(chan struct{})
			client, _ := NewSessionClient[string](testDialer(func(context.Context, string) (Transport, error) {
				close(entered)
				<-release
				return transport, nil
			}), ConnDependencies[string]{Codec: testCodec{}}, ConnConfig{})
			defer client.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { _, err := client.DialContext(ctx, "unused"); result <- err }()
			testReceive(t, entered)
			if byClose {
				_ = client.Close()
			} else {
				cancel()
			}
			close(release)
			expected := error(context.Canceled)
			if byClose {
				expected = net.ErrClosed
			}
			if err := testReceive(t, result); !errors.Is(err, expected) {
				t.Fatalf("Dial: %v", err)
			}
			if err := client.Wait(); !errors.Is(err, expected) {
				t.Fatalf("Wait: %v", err)
			}
			testReceive(t, transport.closed)
		})
	}
}

func TestSessionClientCloseCancelsDial(t *testing.T) {
	entered := make(chan struct{})
	client, _ := NewSessionClient[string](testDialer(func(ctx context.Context, _ string) (Transport, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}), ConnDependencies[string]{Codec: testCodec{}}, ConnConfig{})
	defer client.Close()
	result := make(chan error, 1)
	go func() { _, err := client.DialContext(nil, "unused"); result <- err }()
	testReceive(t, entered)
	_ = client.Close()
	if err := testReceive(t, result); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := client.Wait(); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
