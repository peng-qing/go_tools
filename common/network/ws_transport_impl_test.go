package network

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

const websocketTestTimeout = 2 * time.Second

type websocketAcceptResult struct {
	transport Transport
	err       error
}

// newWebsocketTestConfig 创建字段完整的测试配置，避免嵌套配置为空影响测试目标。
func newWebsocketTestConfig() *WebsocketTransportConfig {
	return &WebsocketTransportConfig{
		IO:              &IOConfig{},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		MaxMessageSize:  1 << 20,
		Handshake: &WebsocketHandshakeConfig{
			Timeout: time.Second,
		},
	}
}

// newWebsocketTransportPair 使用本机监听器和拨号器创建 WebSocket 连接对。
func newWebsocketTransportPair(t *testing.T, config *WebsocketTransportConfig) (*WebsocketTransportImpl, *WebsocketTransportImpl) {
	t.Helper()
	listener, err := NewWebsocketListener(&WebsocketListenerConfig{
		Address:               "127.0.0.1:8080",
		Path:                  "/transport",
		Transport:             config,
		MaxPendingConnections: 4,
		MaxConcurrentUpgrades: 4,
		MaxHeaderBytes:        4096,
	})
	if err != nil {
		t.Fatalf("NewWebsocketListener() error = %v", err)
	}

	accepted := make(chan websocketAcceptResult, 1)
	go func() {
		transport, acceptErr := listener.Accept()
		accepted <- websocketAcceptResult{transport: transport, err: acceptErr}
	}()

	address := "ws://" + listener.ListenerAddr().String() + "/transport"
	clientTransport, err := NewWebsocketDialer(config).Dial(context.Background(), address)
	if err != nil {
		_ = listener.Close()
		t.Fatalf("Dial() error = %v", err)
	}
	client := clientTransport.(*WebsocketTransportImpl)

	var server *WebsocketTransportImpl
	select {
	case result := <-accepted:
		if result.err != nil {
			_ = client.Close()
			_ = listener.Close()
			t.Fatalf("Accept() error = %v", result.err)
		}
		server = result.transport.(*WebsocketTransportImpl)
	case <-time.After(websocketTestTimeout):
		_ = client.Close()
		_ = listener.Close()
		t.Fatal("Accept() timed out")
	}

	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		_ = listener.Close()
	})
	return client, server
}

// TestWebsocketTransportWriteAcceptsCompleteMessage 验证 Write 成功代表对端收到完整的单个二进制消息。
func TestWebsocketTransportWriteAcceptsCompleteMessage(t *testing.T) {
	client, server := newWebsocketTransportPair(t, newWebsocketTestConfig())
	expected := bytes.Repeat([]byte("websocket-payload-"), 4096)

	writeErr := client.Write(expected)
	actual, readErr := server.Read()
	t.Logf("actual: message_length=%d, write_err=%v, read_err=%v", len(actual), writeErr, readErr)
	t.Logf("expected: message_length=%d, write_err=<nil>, read_err=<nil>", len(expected))

	if writeErr != nil {
		t.Fatalf("Write() error = %v", writeErr)
	}
	if readErr != nil {
		t.Fatalf("Read() error = %v", readErr)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatal("对端收到的消息与 Write 输入不一致")
	}
}

// TestWebsocketTransportPreservesMessageBoundaries 验证连续写入不会粘成一个消息，也不会拆分消息。
func TestWebsocketTransportPreservesMessageBoundaries(t *testing.T) {
	client, server := newWebsocketTransportPair(t, newWebsocketTestConfig())
	expected := [][]byte{[]byte("first"), bytes.Repeat([]byte{0x5a}, 8192), []byte("third")}

	for _, message := range expected {
		if err := client.Write(message); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
	actual := make([][]byte, 0, len(expected))
	for range expected {
		message, err := server.Read()
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		actual = append(actual, message)
	}

	actualLengths := []int{len(actual[0]), len(actual[1]), len(actual[2])}
	expectedLengths := []int{len(expected[0]), len(expected[1]), len(expected[2])}
	t.Logf("actual: message_count=%d, message_lengths=%v", len(actual), actualLengths)
	t.Logf("expected: message_count=%d, message_lengths=%v", len(expected), expectedLengths)
	for i := range expected {
		if !bytes.Equal(actual[i], expected[i]) {
			t.Fatalf("message[%d] differs from expected", i)
		}
	}
}

// TestWebsocketTransportCloseUnblocksRead 验证主动关闭连接能够解除正在阻塞的 Read。
func TestWebsocketTransportCloseUnblocksRead(t *testing.T) {
	client, _ := newWebsocketTransportPair(t, newWebsocketTestConfig())
	result := make(chan error, 1)
	go func() {
		_, err := client.Read()
		result <- err
	}()

	select {
	case err := <-result:
		t.Fatalf("Read() returned before Close(): %v", err)
	case <-time.After(30 * time.Millisecond):
	}

	closeErr := client.Close()
	select {
	case readErr := <-result:
		t.Logf("actual: close_err=%v, read_err=%v", closeErr, readErr)
		t.Logf("expected: close_err=<nil>, read_err is non-nil")
		if closeErr != nil {
			t.Fatalf("Close() error = %v", closeErr)
		}
		if readErr == nil {
			t.Fatal("Read() error = nil after Close()")
		}
	case <-time.After(websocketTestTimeout):
		t.Fatal("Close() did not unblock Read()")
	}
}

// TestWebsocketTransportCloseUnblocksWrite 验证主动关闭连接能够解除被底层发送缓冲区阻塞的 Write。
func TestWebsocketTransportCloseUnblocksWrite(t *testing.T) {
	client, _ := newWebsocketTransportPair(t, newWebsocketTestConfig())
	if tcpConn, ok := client.conn.NetConn().(*net.TCPConn); ok {
		if err := tcpConn.SetWriteBuffer(1024); err != nil {
			t.Fatalf("SetWriteBuffer() error = %v", err)
		}
	}

	result := make(chan error, 1)
	go func() {
		result <- client.Write(make([]byte, 32<<20))
	}()

	select {
	case err := <-result:
		t.Fatalf("Write() returned before Close(): %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	closeErr := client.Close()
	select {
	case writeErr := <-result:
		t.Logf("actual: close_err=%v, write_err=%v", closeErr, writeErr)
		t.Logf("expected: close_err=<nil>, write_err is non-nil")
		if closeErr != nil {
			t.Fatalf("Close() error = %v", closeErr)
		}
		if writeErr == nil {
			t.Fatal("Write() error = nil after Close() interrupted it")
		}
	case <-time.After(websocketTestTimeout):
		t.Fatal("Close() did not unblock Write()")
	}
}

// TestWebsocketTransportReadResults 验证超时、对端正常关闭和主动关闭都有明确结果。
func TestWebsocketTransportReadResults(t *testing.T) {
	t.Run("读取超时", func(t *testing.T) {
		config := newWebsocketTestConfig()
		config.IO.ReadTimeout = 30 * time.Millisecond
		client, _ := newWebsocketTransportPair(t, config)

		actual, actualErr := client.Read()
		var netErr net.Error
		isTimeout := errors.As(actualErr, &netErr) && netErr.Timeout()
		t.Logf("actual: data=%q, err=%v, net_error_timeout=%v", actual, actualErr, isTimeout)
		t.Logf("expected: data=[], err implements net.Error and Timeout()=true")
		if len(actual) != 0 || !isTimeout {
			t.Fatalf("Read() = %q, %v; want empty data and timeout", actual, actualErr)
		}
	})

	t.Run("对端正常关闭", func(t *testing.T) {
		client, server := newWebsocketTransportPair(t, newWebsocketTestConfig())
		if err := server.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"),
			time.Now().Add(time.Second),
		); err != nil {
			t.Fatalf("WriteControl(CloseMessage) error = %v", err)
		}

		actual, actualErr := client.Read()
		isNormalClose := websocket.IsCloseError(actualErr, websocket.CloseNormalClosure)
		t.Logf("actual: data=%q, err=%v, normal_close=%v", actual, actualErr, isNormalClose)
		t.Logf("expected: data=[], err is websocket close code %d", websocket.CloseNormalClosure)
		if len(actual) != 0 || !isNormalClose {
			t.Fatalf("Read() = %q, %v; want normal close error", actual, actualErr)
		}
	})

	t.Run("主动关闭", func(t *testing.T) {
		client, _ := newWebsocketTransportPair(t, newWebsocketTestConfig())
		if err := client.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		actual, actualErr := client.Read()
		t.Logf("actual: data=%q, err=%v", actual, actualErr)
		t.Logf("expected: data=[], err is non-nil")
		if len(actual) != 0 || actualErr == nil {
			t.Fatalf("Read() = %q, %v; want empty data and close error", actual, actualErr)
		}
	})
}

// TestWebsocketTransportRejectsTextMessage 验证文本消息不会被误当作二进制业务数据。
func TestWebsocketTransportRejectsTextMessage(t *testing.T) {
	client, server := newWebsocketTransportPair(t, newWebsocketTestConfig())
	expectedText := "not binary"
	if err := client.conn.WriteMessage(websocket.TextMessage, []byte(expectedText)); err != nil {
		t.Fatalf("WriteMessage(TextMessage) error = %v", err)
	}

	actual, actualErr := server.Read()
	t.Logf("actual: data=%q, err=%v, errors.Is(ErrNonBinaryMessage)=%v", actual, actualErr, errors.Is(actualErr, ErrNonBinaryMessage))
	t.Logf("expected: data=[], err wraps ErrNonBinaryMessage")
	if len(actual) != 0 {
		t.Fatalf("Read() data = %q, want empty", actual)
	}
	if !errors.Is(actualErr, ErrNonBinaryMessage) {
		t.Fatalf("Read() error = %v, want ErrNonBinaryMessage", actualErr)
	}
}

// TestWebsocketTransportEnforcesMaxMessageSize 验证超大消息被拒绝，不会作为业务数据返回。
func TestWebsocketTransportEnforcesMaxMessageSize(t *testing.T) {
	config := newWebsocketTestConfig()
	config.MaxMessageSize = 32
	client, server := newWebsocketTransportPair(t, config)
	oversized := bytes.Repeat([]byte{0x7f}, 33)
	if err := client.Write(oversized); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	actual, actualErr := server.Read()
	containsLimitError := actualErr != nil && strings.Contains(actualErr.Error(), "read limit")
	t.Logf("actual: data_length=%d, err=%v, contains_read_limit=%v", len(actual), actualErr, containsLimitError)
	t.Logf("expected: data_length=0, err reports websocket read limit")
	if len(actual) != 0 || !containsLimitError {
		t.Fatalf("Read() = length %d, %v; want read-limit error", len(actual), actualErr)
	}
}

// TestWebsocketListenerDialerBidirectional 验证监听、拨号、地址、传输模式及双向消息读写。
func TestWebsocketListenerDialerBidirectional(t *testing.T) {
	client, server := newWebsocketTransportPair(t, newWebsocketTestConfig())
	t.Logf("actual: client_local=%v, client_remote=%v", client.LocalAddr(), client.RemoteAddr())
	t.Logf("expected: all addresses are non-nil")
	if client.LocalAddr() == nil || client.RemoteAddr() == nil || server.LocalAddr() == nil || server.RemoteAddr() == nil {
		t.Fatal("transport returned a nil address")
	}

	clientPayload := []byte("client to server")
	if err := client.Write(clientPayload); err != nil {
		t.Fatalf("client Write() error = %v", err)
	}
	serverData, serverErr := server.Read()
	t.Logf("actual: server_received=%q, err=%v", serverData, serverErr)
	t.Logf("expected: server_received=%q, err=<nil>", clientPayload)
	if serverErr != nil || !bytes.Equal(serverData, clientPayload) {
		t.Fatalf("server Read() = %q, %v", serverData, serverErr)
	}

	serverPayload := []byte("server to client")
	if err := server.Write(serverPayload); err != nil {
		t.Fatalf("server Write() error = %v", err)
	}
	clientData, clientErr := client.Read()
	t.Logf("actual: client_received=%q, err=%v", clientData, clientErr)
	t.Logf("expected: client_received=%q, err=<nil>", serverPayload)
	if clientErr != nil || !bytes.Equal(clientData, serverPayload) {
		t.Fatalf("client Read() = %q, %v", clientData, clientErr)
	}
}

// TestWebsocketListenerCloseUnblocksAccept 验证关闭监听器解除 Accept 阻塞，并且重复关闭结果一致。
func TestWebsocketListenerCloseUnblocksAccept(t *testing.T) {
	config := newWebsocketTestConfig()
	listener, err := NewWebsocketListener(&WebsocketListenerConfig{
		Address:               "127.0.0.1:0",
		Transport:             config,
		MaxPendingConnections: 1,
		MaxConcurrentUpgrades: 1,
		MaxHeaderBytes:        4096,
	})
	if err != nil {
		t.Fatalf("NewWebsocketListener() error = %v", err)
	}

	result := make(chan error, 1)
	go func() {
		_, acceptErr := listener.Accept()
		result <- acceptErr
	}()
	select {
	case acceptErr := <-result:
		t.Fatalf("Accept() returned before Close(): %v", acceptErr)
	case <-time.After(30 * time.Millisecond):
	}

	firstCloseErr := listener.Close()
	secondCloseErr := listener.Close()
	select {
	case acceptErr := <-result:
		t.Logf("actual: first_close_err=%v, second_close_err=%v, accept_err=%v", firstCloseErr, secondCloseErr, acceptErr)
		t.Logf("expected: first_close_err=<nil>, second_close_err=<nil>, accept_err wraps ErrClosed")
		if firstCloseErr != nil || secondCloseErr != nil {
			t.Fatalf("Close() errors = %v, %v", firstCloseErr, secondCloseErr)
		}
		if !errors.Is(acceptErr, ErrClosed) {
			t.Fatalf("Accept() error = %v, want ErrClosed", acceptErr)
		}
	case <-time.After(websocketTestTimeout):
		t.Fatal("Close() did not unblock Accept()")
	}
}

// TestWebsocketDialerRejectsInvalidRequests 验证取消上下文和非 WebSocket 地址均返回明确错误。
func TestWebsocketDialerRejectsInvalidRequests(t *testing.T) {
	dialer := NewWebsocketDialer(newWebsocketTestConfig())
	t.Run("取消上下文", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		transport, err := dialer.Dial(ctx, "ws://127.0.0.1:1/")
		t.Logf("actual: transport=%v, err=%v", transport, err)
		t.Logf("expected: transport=<nil>, err wraps context.Canceled")
		if transport != nil {
			_ = transport.Close()
			t.Fatal("Dial() returned transport for canceled context")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Dial() error = %v, want context.Canceled", err)
		}
	})

	t.Run("错误协议", func(t *testing.T) {
		transport, err := dialer.Dial(context.Background(), "http://127.0.0.1/")
		t.Logf("actual: transport=%v, err=%v", transport, err)
		t.Logf("expected: transport=<nil>, err reports ws or wss requirement")
		if transport != nil {
			_ = transport.Close()
			t.Fatal("Dial() returned transport for http address")
		}
		if err == nil {
			t.Fatal("Dial() error = nil for http address")
		}
	})
}

// TestWebsocketListenerRejectsWrongPath 验证监听器只在配置路径执行协议升级。
func TestWebsocketListenerRejectsWrongPath(t *testing.T) {
	config := newWebsocketTestConfig()
	listener, err := NewWebsocketListener(&WebsocketListenerConfig{
		Address:               "127.0.0.1:0",
		Path:                  "/correct",
		Transport:             config,
		MaxPendingConnections: 1,
		MaxConcurrentUpgrades: 1,
		MaxHeaderBytes:        4096,
	})
	if err != nil {
		t.Fatalf("NewWebsocketListener() error = %v", err)
	}
	defer listener.Close()

	address := "ws://" + listener.ListenerAddr().String() + "/wrong"
	conn, response, dialErr := websocket.DefaultDialer.Dial(address, http.Header{})
	if conn != nil {
		_ = conn.Close()
	}
	status := 0
	if response != nil {
		status = response.StatusCode
		_ = response.Body.Close()
	}
	t.Logf("actual: conn_nil=%v, status=%d, err=%v", conn == nil, status, dialErr)
	t.Logf("expected: conn_nil=true, status=%d, err is non-nil", http.StatusNotFound)
	if conn != nil || dialErr == nil || status != http.StatusNotFound {
		t.Fatalf("wrong-path Dial() = conn %v, status %d, err %v", conn, status, dialErr)
	}
}

// wsAwait 为所有异步断言设置上限，避免回归导致测试永久阻塞。
func wsAwait[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(websocketTestTimeout):
		t.Fatal("等待异步操作超时")
	}
	var zero T
	return zero
}

func wsReviewListener(t *testing.T) *WebsocketListenerImpl {
	t.Helper()
	l, err := NewWebsocketListener(&WebsocketListenerConfig{
		Address: "127.0.0.1:0", Path: "/allowed", Transport: newWebsocketTestConfig(),
		MaxPendingConnections: 1,
		MaxConcurrentUpgrades: 1,
		MaxHeaderBytes:        4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func wsReviewDial(t *testing.T, l *WebsocketListenerImpl) *websocket.Conn {
	t.Helper()
	d := websocket.Dialer{HandshakeTimeout: time.Second}
	c, r, err := d.Dial("ws://"+l.ListenerAddr().String()+"/allowed", nil)
	if r != nil && r.Body != nil {
		_ = r.Body.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// wsWaitQueued 只观察队列，不提前取走连接，确保真正触发待交付清理路径。
func wsWaitQueued(t *testing.T, l *WebsocketListenerImpl) {
	t.Helper()
	deadline := time.NewTimer(websocketTestTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for len(l.acceptQueue) == 0 {
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("连接未进入接收队列")
		}
	}
}

func wsPeerClosed(t *testing.T, c *websocket.Conn) {
	t.Helper()
	if err := c.SetReadDeadline(time.Now().Add(websocketTestTimeout)); err != nil {
		t.Fatal(err)
	}
	_, data, err := c.ReadMessage()
	var ne net.Error
	timeout := errors.As(err, &ne) && ne.Timeout()
	t.Logf("actual: data=%q, err=%v, timeout=%v", data, err, timeout)
	t.Log("expected: data为空, err非空, timeout=false")
	if err == nil || timeout || len(data) != 0 {
		t.Fatal("连接未及时释放")
	}
}

// TestWebsocketTerminalBroadcast 验证主动关闭和底层异常均广播给所有等待者并保留首次原因。
func TestWebsocketTerminalBroadcast(t *testing.T) {
	for _, failure := range []bool{false, true} {
		name := "主动关闭"
		if failure {
			name = "底层异常"
		}
		t.Run(name, func(t *testing.T) {
			l := wsReviewListener(t)
			results := make(chan error, 16)
			for i := 0; i < 16; i++ {
				go func() {
					c, e := l.Accept()
					if c != nil {
						_ = c.Close()
					}
					results <- e
				}()
			}
			if failure {
				if err := l.listener.Close(); err != nil {
					t.Fatal(err)
				}
			} else if err := l.Close(); err != nil {
				t.Fatal(err)
			}
			var cause error
			for i := 0; i < 16; i++ {
				err := wsAwait(t, results)
				if i == 0 {
					cause = err
				}
				t.Logf("actual: waiter=%d, err=%v, same_cause=%v", i, err, err == cause)
				t.Logf("expected: waiter=%d, err非空, same_cause=true", i)
				if err == nil || err != cause {
					t.Fatal("终止原因不稳定")
				}
				if failure {
					var op *net.OpError
					if !errors.As(err, &op) || op.Op != "accept" {
						t.Fatalf("丢失底层异常: %v", err)
					}
				} else if !errors.Is(err, ErrClosed) {
					t.Fatal(err)
				}
			}
			closed := make(chan error, 8)
			for i := 0; i < 8; i++ {
				go func() { closed <- l.Close() }()
			}
			for i := 0; i < 8; i++ {
				if e := wsAwait(t, closed); e != nil {
					t.Fatal(e)
				}
			}
			for i := 0; i < 3; i++ {
				go func() { _, e := l.Accept(); results <- e }()
				e := wsAwait(t, results)
				t.Logf("actual: subsequent_accept_err=%v", e)
				t.Logf("expected: subsequent_accept_err=%v", cause)
				if e != cause {
					t.Fatal("后续调用丢失首次原因")
				}
			}
		})
	}
}

// TestWebsocketQueuedConnectionCleanup 验证主动关闭及异常退出都会释放未交付连接。
func TestWebsocketQueuedConnectionCleanup(t *testing.T) {
	for _, failure := range []bool{false, true} {
		name := "主动关闭"
		if failure {
			name = "底层异常"
		}
		t.Run(name, func(t *testing.T) {
			l := wsReviewListener(t)
			c := wsReviewDial(t, l)
			wsWaitQueued(t, l)
			if failure {
				if e := l.listener.Close(); e != nil {
					t.Fatal(e)
				}
				wsAwait(t, l.done)
			}
			if e := l.Close(); e != nil {
				t.Fatal(e)
			}
			t.Logf("actual: queued=%d", len(l.acceptQueue))
			t.Log("expected: queued=0")
			if len(l.acceptQueue) != 0 {
				t.Fatal("队列未清空")
			}
			wsPeerClosed(t, c)
		})
	}
}

// TestWebsocketQueueFullRecovery 验证队列满时关闭新连接，释放容量后仍可正常接收。
func TestWebsocketQueueFullRecovery(t *testing.T) {
	l := wsReviewListener(t)
	_ = wsReviewDial(t, l)
	wsWaitQueued(t, l)
	rejected := wsReviewDial(t, l)
	wsPeerClosed(t, rejected)
	first, e := l.Accept()
	if e != nil {
		t.Fatal(e)
	}
	defer first.Close()
	peer := wsReviewDial(t, l)
	wsWaitQueued(t, l)
	next, e := l.Accept()
	if e != nil {
		t.Fatal(e)
	}
	defer next.Close()
	if e := peer.WriteMessage(websocket.BinaryMessage, []byte("after-full")); e != nil {
		t.Fatal(e)
	}
	_ = next.(*WebsocketTransportImpl).conn.SetReadDeadline(time.Now().Add(websocketTestTimeout))
	data, e := next.Read()
	t.Logf("actual: data=%q, err=%v", data, e)
	t.Log("expected: data=\"after-full\", err=<nil>")
	if e != nil || string(data) != "after-full" {
		t.Fatal("容量释放后通信失败")
	}
}

// TestWebsocketRejectedRequestRecovery 验证错误路径及无效升级请求不会停止服务。
func TestWebsocketRejectedRequestRecovery(t *testing.T) {
	l := wsReviewListener(t)
	client := http.Client{Timeout: time.Second}
	for _, tc := range []struct {
		path   string
		status int
	}{{"/wrong", 404}, {"/allowed", 400}} {
		r, e := client.Get("http://" + l.ListenerAddr().String() + tc.path)
		if e != nil {
			t.Fatal(e)
		}
		_ = r.Body.Close()
		t.Logf("actual: path=%s, status=%d", tc.path, r.StatusCode)
		t.Logf("expected: path=%s, status=%d", tc.path, tc.status)
		if r.StatusCode != tc.status {
			t.Fatal("拒绝状态错误")
		}
	}
	_ = wsReviewDial(t, l)
	wsWaitQueued(t, l)
	c, e := l.Accept()
	if c != nil {
		defer c.Close()
	}
	t.Logf("actual: accepted=%v, err=%v", c != nil, e)
	t.Log("expected: accepted=true, err=<nil>")
	if c == nil || e != nil {
		t.Fatal("非法请求影响后续接收")
	}
}

// wsGatedHandshake 在 Hijack 后阻塞握手写入，精确控制升级和关闭的交错顺序。
type wsGatedHandshake struct {
	net.Conn
	entered, release, closed chan struct{}
	enterOnce, closeOnce     sync.Once
}

func (c *wsGatedHandshake) Write(p []byte) (int, error) {
	c.enterOnce.Do(func() { close(c.entered) })
	select {
	case <-c.release:
		return len(p), nil
	case <-c.closed:
		return 0, net.ErrClosed
	}
}
func (c *wsGatedHandshake) Close() error {
	c.closeOnce.Do(func() { close(c.closed); _ = c.Conn.Close() })
	return nil
}

type wsHijacker struct {
	conn   net.Conn
	header http.Header
}

func (w *wsHijacker) Header() http.Header       { return w.header }
func (w *wsHijacker) WriteHeader(int)           {}
func (w *wsHijacker) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func (w *wsHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, bufio.NewReadWriter(bufio.NewReader(w.conn), bufio.NewWriter(w.conn)), nil
}

// TestWebsocketUpgradeCloseAndAdmission 验证升级槽位拒绝、恢复以及关闭等待已 Hijack 的连接。
func TestWebsocketUpgradeCloseAndAdmission(t *testing.T) {
	l := wsReviewListener(t)
	raw, peer := net.Pipe()
	defer peer.Close()
	c := &wsGatedHandshake{Conn: raw, entered: make(chan struct{}), release: make(chan struct{}), closed: make(chan struct{})}
	var once sync.Once
	release := func() { once.Do(func() { close(c.release) }) }
	defer func() { release(); _ = c.Close() }()
	req := httptest.NewRequest(http.MethodGet, "http://example.test/allowed", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	finished := make(chan struct{})
	go func() { defer close(finished); l.handleUpgrade(&wsHijacker{c, make(http.Header)}, req) }()
	wsAwait(t, c.entered)
	recorder := httptest.NewRecorder()
	l.handleUpgrade(recorder, req.Clone(req.Context()))
	t.Logf("actual: admission_status=%d", recorder.Code)
	t.Log("expected: admission_status=503")
	if recorder.Code != 503 {
		t.Fatal("槽位已满仍接受请求")
	}
	closed := make(chan error, 1)
	go func() { closed <- l.Close() }()
	wsAwait(t, l.done)
	select {
	case e := <-closed:
		t.Fatalf("升级结束前提前关闭: %v", e)
	case <-time.After(20 * time.Millisecond):
	}
	release()
	wsAwait(t, finished)
	e := wsAwait(t, closed)
	wsAwait(t, c.closed)
	t.Logf("actual: close_err=%v, queued=%d, slots=%d", e, len(l.acceptQueue), len(l.upgradeSlots))
	t.Log("expected: close_err=<nil>, queued=0, slots=0")
	if e != nil || len(l.acceptQueue) != 0 || len(l.upgradeSlots) != 0 {
		t.Fatal("升级资源未释放")
	}
	recorder = httptest.NewRecorder()
	l.handleUpgrade(recorder, req)
	t.Logf("actual: closed_listener_status=%d", recorder.Code)
	t.Log("expected: closed_listener_status=503")
	if recorder.Code != 503 {
		t.Fatal("关闭后仍处理升级")
	}
}

// TestWebsocketWriteTimeout 验证对端不读取时写入超时；底层写失败后不能假报后续写入成功。
func TestWebsocketWriteTimeout(t *testing.T) {
	config := newWebsocketTestConfig()
	config.IO.WriteTimeout = 40 * time.Millisecond
	c, _ := newWebsocketTransportPair(t, config)
	tcp, ok := c.conn.NetConn().(*net.TCPConn)
	if !ok {
		t.Fatal("底层不是TCP")
	}
	if e := tcp.SetWriteBuffer(1024); e != nil {
		t.Fatal(e)
	}
	result := make(chan error, 1)
	go func() { result <- c.Write(make([]byte, 8<<20)) }()
	e := wsAwait(t, result)
	var ne net.Error
	timeout := errors.As(e, &ne) && ne.Timeout()
	t.Logf("actual: err=%v, timeout=%v", e, timeout)
	t.Log("expected: err非空, timeout=true")
	if !timeout {
		t.Fatal("未返回写超时")
	}
	e = c.Write([]byte("after-timeout"))
	t.Logf("actual: subsequent_write_err=%v", e)
	t.Log("expected: subsequent_write_err非空")
	if e == nil {
		t.Fatal("失败连接仍报告写成功")
	}
}

// TestWebsocketBoundaryAndFinalMessage 验证空消息、最大合法消息以及关闭前的最后一条完整消息。
func TestWebsocketBoundaryAndFinalMessage(t *testing.T) {
	cfg := newWebsocketTestConfig()
	cfg.MaxMessageSize = 32
	cfg.IO.ReadTimeout = time.Second
	c, s := newWebsocketTransportPair(t, cfg)
	for _, want := range [][]byte{{}, bytes.Repeat([]byte("x"), 32), []byte("last")} {
		if e := s.Write(want); e != nil {
			t.Fatal(e)
		}
		got, e := c.Read()
		t.Logf("actual: data=%q, err=%v", got, e)
		t.Logf("expected: data=%q, err=<nil>", want)
		if e != nil || !bytes.Equal(got, want) {
			t.Fatal("消息内容不一致")
		}
	}
	if e := s.Write([]byte("before-close")); e != nil {
		t.Fatal(e)
	}
	if e := s.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(1000, "done"), time.Now().Add(time.Second)); e != nil {
		t.Fatal(e)
	}
	got, e := c.Read()
	t.Logf("actual: final_data=%q, err=%v", got, e)
	t.Log("expected: final_data=\"before-close\", err=<nil>")
	if e != nil || string(got) != "before-close" {
		t.Fatal("完整尾消息丢失")
	}
	_, e = c.Read()
	var ce *websocket.CloseError
	normal := errors.As(e, &ce) && ce.Code == 1000
	t.Logf("actual: close_err=%v, normal=%v", e, normal)
	t.Log("expected: normal=true, close_code=1000")
	if !normal {
		t.Fatal("关闭结果不明确")
	}
}

// TestWebsocketConcurrentTransportClose 验证并发关闭返回一致结果且解除读取阻塞。
func TestWebsocketConcurrentTransportClose(t *testing.T) {
	c, _ := newWebsocketTransportPair(t, newWebsocketTestConfig())
	read := make(chan error, 1)
	go func() { _, e := c.Read(); read <- e }()
	closed := make(chan error, 16)
	for i := 0; i < 16; i++ {
		go func() { closed <- c.Close() }()
	}
	for i := 0; i < 16; i++ {
		e := wsAwait(t, closed)
		t.Logf("actual: close[%d]=%v", i, e)
		t.Logf("expected: close[%d]=<nil>", i)
		if e != nil {
			t.Fatal(e)
		}
	}
	e := wsAwait(t, read)
	t.Logf("actual: read_err=%v", e)
	t.Log("expected: read_err非空")
	if e == nil {
		t.Fatal("关闭后读取未失败")
	}
}
