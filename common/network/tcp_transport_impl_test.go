package network

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

const transportTestTimeout = 2 * time.Second

type tcpReadResult struct {
	data []byte
	err  error
}

// newTcpTransportPair 创建仅监听本机回环地址的 TCP 连接对，不需要完整服务端。
func newTcpTransportPair(t *testing.T, config *TcpTransportConfig) (*TcpTransportImpl, *net.TCPConn) {
	t.Helper()
	if config.IO == nil {
		// 测试使用有效的完整配置，避免配置结构调整后因缺少嵌套 IO 配置而触发空指针。
		configCopy := *config
		configCopy.IO = &IOConfig{}
		config = &configCopy
	}

	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 0,
	})
	if err != nil {
		t.Fatalf("ListenTCP() error = %v", err)
	}

	accepted := make(chan struct {
		conn *net.TCPConn
		err  error
	}, 1)
	go func() {
		conn, acceptErr := listener.AcceptTCP()
		accepted <- struct {
			conn *net.TCPConn
			err  error
		}{conn: conn, err: acceptErr}
	}()

	client, err := net.DialTCP("tcp4", nil, listener.Addr().(*net.TCPAddr))
	if err != nil {
		_ = listener.Close()
		t.Fatalf("DialTCP() error = %v", err)
	}

	var server *net.TCPConn
	select {
	case result := <-accepted:
		if result.err != nil {
			_ = client.Close()
			_ = listener.Close()
			t.Fatalf("AcceptTCP() error = %v", result.err)
		}
		server = result.conn
	case <-time.After(transportTestTimeout):
		_ = client.Close()
		_ = listener.Close()
		t.Fatal("AcceptTCP() timed out")
	}

	transport, err := newTcpTransportImpl(client, config)
	if err != nil {
		_ = client.Close()
		_ = server.Close()
		_ = listener.Close()
		t.Fatalf("newTcpTransportImpl() error = %v", err)
	}

	t.Cleanup(func() {
		_ = transport.Close()
		_ = server.Close()
		_ = listener.Close()
	})
	return transport, server
}

// TestTcpTransportWriteAcceptsAllBytes 验证 Write 成功返回时，对端能够收到全部字节。
func TestTcpTransportWriteAcceptsAllBytes(t *testing.T) {
	transport, peer := newTcpTransportPair(t, &TcpTransportConfig{})
	expected := bytes.Repeat([]byte("tcp-write-payload-"), 16*1024)

	received := make(chan tcpReadResult, 1)
	go func() {
		actual := make([]byte, len(expected))
		_, err := io.ReadFull(peer, actual)
		received <- tcpReadResult{data: actual, err: err}
	}()

	writeErr := transport.Write(expected)
	select {
	case result := <-received:
		t.Logf("actual: received_length=%d, write_err=%v, read_err=%v", len(result.data), writeErr, result.err)
		t.Logf("expected: received_length=%d, write_err=<nil>, read_err=<nil>", len(expected))

		if writeErr != nil {
			t.Fatalf("Write() error = %v", writeErr)
		}
		if result.err != nil {
			t.Fatalf("peer ReadFull() error = %v", result.err)
		}
		if !bytes.Equal(result.data, expected) {
			t.Fatal("peer received data differs from Write input")
		}
	case <-time.After(transportTestTimeout):
		t.Fatal("peer did not receive all bytes before timeout")
	}
}

// TestTcpTransportCloseUnblocksReadAndWrite 验证主动关闭连接能够解除正在阻塞的 Read 和 Write。
func TestTcpTransportCloseUnblocksReadAndWrite(t *testing.T) {
	t.Run("解除阻塞的Read", func(t *testing.T) {
		transport, _ := newTcpTransportPair(t, &TcpTransportConfig{ReadBufferSize: 64})
		result := make(chan error, 1)
		go func() {
			_, err := transport.Read()
			result <- err
		}()

		select {
		case err := <-result:
			t.Fatalf("Read() returned before Close(): %v", err)
		case <-time.After(30 * time.Millisecond):
		}

		closeErr := transport.Close()
		select {
		case actualErr := <-result:
			t.Logf("actual: close_err=%v, read_err=%v", closeErr, actualErr)
			t.Logf("expected: close_err=<nil>, read_err wraps net.ErrClosed")
			if closeErr != nil {
				t.Fatalf("Close() error = %v", closeErr)
			}
			if !errors.Is(actualErr, net.ErrClosed) {
				t.Fatalf("Read() error = %v, want error wrapping net.ErrClosed", actualErr)
			}
		case <-time.After(transportTestTimeout):
			t.Fatal("Close() did not unblock Read()")
		}
	})

	t.Run("解除阻塞的Write", func(t *testing.T) {
		transport, _ := newTcpTransportPair(t, &TcpTransportConfig{})
		if err := transport.conn.SetWriteBuffer(1024); err != nil {
			t.Fatalf("SetWriteBuffer() error = %v", err)
		}

		result := make(chan error, 1)
		go func() {
			result <- transport.Write(make([]byte, 32<<20))
		}()

		select {
		case err := <-result:
			t.Fatalf("Write() returned before Close(): %v", err)
		case <-time.After(50 * time.Millisecond):
		}

		closeErr := transport.Close()
		select {
		case actualErr := <-result:
			t.Logf("actual: close_err=%v, write_err=%v", closeErr, actualErr)
			t.Logf("expected: close_err=<nil>, write_err wraps net.ErrClosed")
			if closeErr != nil {
				t.Fatalf("Close() error = %v", closeErr)
			}
			if !errors.Is(actualErr, net.ErrClosed) {
				t.Fatalf("Write() error = %v, want error wrapping net.ErrClosed", actualErr)
			}
		case <-time.After(transportTestTimeout):
			t.Fatal("Close() did not unblock Write()")
		}
	})
}

// TestTcpTransportReadPreservesFinalData 验证对端发送最后一段数据后关闭写端时，数据不会因 EOF 被丢弃。
func TestTcpTransportReadPreservesFinalData(t *testing.T) {
	transport, peer := newTcpTransportPair(t, &TcpTransportConfig{ReadBufferSize: 64})
	expectedData := []byte("last bytes before EOF")

	peerResult := make(chan error, 1)
	go func() {
		if _, err := peer.Write(expectedData); err != nil {
			peerResult <- err
			return
		}
		peerResult <- peer.CloseWrite()
	}()

	actualData, firstErr := transport.Read()
	var eofErr error
	if firstErr == nil {
		_, eofErr = transport.Read()
	} else {
		eofErr = firstErr
	}

	t.Logf("actual: first_data=%q, first_err=%v, eof_err=%v", actualData, firstErr, eofErr)
	t.Logf("expected: first_data=%q, first_err=<nil>或EOF, eof_err=EOF", expectedData)

	if !bytes.Equal(actualData, expectedData) {
		t.Fatalf("Read() data = %q, want %q", actualData, expectedData)
	}
	if firstErr != nil && !errors.Is(firstErr, io.EOF) {
		t.Fatalf("first Read() error = %v, want nil or EOF", firstErr)
	}
	if !errors.Is(eofErr, io.EOF) {
		t.Fatalf("final Read() error = %v, want EOF", eofErr)
	}
	select {
	case err := <-peerResult:
		if err != nil {
			t.Fatalf("peer write/CloseWrite error = %v", err)
		}
	case <-time.After(transportTestTimeout):
		t.Fatal("peer write did not finish")
	}
}

// TestTcpTransportReadResults 验证读取超时、对端 EOF 和主动关闭都有可识别且稳定的结果。
func TestTcpTransportReadResults(t *testing.T) {
	t.Run("读取超时", func(t *testing.T) {
		transport, _ := newTcpTransportPair(t, &TcpTransportConfig{
			ReadBufferSize: 64,
			IO: &IOConfig{
				ReadTimeout: 30 * time.Millisecond,
			},
		})

		actualData, actualErr := transport.Read()
		isTimeout := errors.Is(actualErr, os.ErrDeadlineExceeded)
		t.Logf("actual: data=%q, err=%v, errors.Is(os.ErrDeadlineExceeded)=%v", actualData, actualErr, isTimeout)
		t.Logf("expected: data=[], err wraps os.ErrDeadlineExceeded")

		if len(actualData) != 0 {
			t.Fatalf("Read() data = %q, want empty", actualData)
		}
		if !isTimeout {
			t.Fatalf("Read() error = %v, want timeout error", actualErr)
		}
	})

	t.Run("对端EOF", func(t *testing.T) {
		transport, peer := newTcpTransportPair(t, &TcpTransportConfig{ReadBufferSize: 64})
		if err := peer.CloseWrite(); err != nil {
			t.Fatalf("peer.CloseWrite() error = %v", err)
		}

		actualData, actualErr := transport.Read()
		t.Logf("actual: data=%q, err=%v", actualData, actualErr)
		t.Logf("expected: data=[], err=%v", io.EOF)

		if len(actualData) != 0 {
			t.Fatalf("Read() data = %q, want empty", actualData)
		}
		if !errors.Is(actualErr, io.EOF) {
			t.Fatalf("Read() error = %v, want %v", actualErr, io.EOF)
		}
	})

	t.Run("主动关闭", func(t *testing.T) {
		transport, _ := newTcpTransportPair(t, &TcpTransportConfig{ReadBufferSize: 64})
		if err := transport.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		actualData, actualErr := transport.Read()
		t.Logf("actual: data=%q, err=%v", actualData, actualErr)
		t.Logf("expected: data=[], err wraps net.ErrClosed")

		if len(actualData) != 0 {
			t.Fatalf("Read() data = %q, want empty", actualData)
		}
		if !errors.Is(actualErr, net.ErrClosed) {
			t.Fatalf("Read() error = %v, want error wrapping net.ErrClosed", actualErr)
		}
	})
}

// TestTcpListenerDialerBidirectional 验证监听、拨号、地址信息、传输模式以及双向读写。
func TestTcpListenerDialerBidirectional(t *testing.T) {
	config := &TcpTransportConfig{
		ReadBufferSize: 1024,
		IO:             &IOConfig{},
	}
	listener, err := NewTcpListener("127.0.0.1:0", config)
	if err != nil {
		t.Fatalf("NewTcpListener() error = %v", err)
	}
	defer listener.Close()

	accepted := make(chan struct {
		transport Transport
		err       error
	}, 1)
	go func() {
		transport, acceptErr := listener.Accept()
		accepted <- struct {
			transport Transport
			err       error
		}{transport: transport, err: acceptErr}
	}()

	client, err := NewTcpDialer(config).Dial(context.Background(), listener.ListenerAddr().String())
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer client.Close()

	var server Transport
	select {
	case result := <-accepted:
		if result.err != nil {
			t.Fatalf("Accept() error = %v", result.err)
		}
		server = result.transport
	case <-time.After(transportTestTimeout):
		t.Fatal("Accept() timed out")
	}
	defer server.Close()

	t.Logf(
		"actual: listener_addr=%v, client_local=%v, client_remote=%v",
		listener.ListenerAddr(),
		client.LocalAddr(),
		client.RemoteAddr(),
	)
	t.Logf("expected: all addresses are non-nil")
	if listener.ListenerAddr() == nil || client.LocalAddr() == nil || client.RemoteAddr() == nil ||
		server.LocalAddr() == nil || server.RemoteAddr() == nil {
		t.Fatal("listener or transport returned a nil address")
	}

	clientPayload := []byte("client to server")
	if err := client.Write(clientPayload); err != nil {
		t.Fatalf("client Write() error = %v", err)
	}
	serverData, err := server.Read()
	t.Logf("actual: server_received=%q, err=%v", serverData, err)
	t.Logf("expected: server_received=%q, err=<nil>", clientPayload)
	if err != nil || !bytes.Equal(serverData, clientPayload) {
		t.Fatalf("server Read() = %q, %v", serverData, err)
	}

	serverPayload := []byte("server to client")
	if err := server.Write(serverPayload); err != nil {
		t.Fatalf("server Write() error = %v", err)
	}
	clientData, err := client.Read()
	t.Logf("actual: client_received=%q, err=%v", clientData, err)
	t.Logf("expected: client_received=%q, err=<nil>", serverPayload)
	if err != nil || !bytes.Equal(clientData, serverPayload) {
		t.Fatalf("client Read() = %q, %v", clientData, err)
	}
}

// TestTcpListenerCloseUnblocksAccept 验证关闭监听器能够解除阻塞的 Accept，并且重复关闭结果一致。
func TestTcpListenerCloseUnblocksAccept(t *testing.T) {
	listener, err := NewTcpListener("127.0.0.1:0", &TcpTransportConfig{ReadBufferSize: 64})
	if err != nil {
		t.Fatalf("NewTcpListener() error = %v", err)
	}

	result := make(chan error, 1)
	go func() {
		_, acceptErr := listener.Accept()
		result <- acceptErr
	}()

	select {
	case err := <-result:
		t.Fatalf("Accept() returned before Close(): %v", err)
	case <-time.After(30 * time.Millisecond):
	}

	firstCloseErr := listener.Close()
	secondCloseErr := listener.Close()
	select {
	case acceptErr := <-result:
		t.Logf("actual: first_close_err=%v, second_close_err=%v, accept_err=%v", firstCloseErr, secondCloseErr, acceptErr)
		t.Logf("expected: first_close_err=<nil>, second_close_err=<nil>, accept_err wraps net.ErrClosed")
		if firstCloseErr != nil || secondCloseErr != nil {
			t.Fatalf("Listener Close() errors = %v, %v", firstCloseErr, secondCloseErr)
		}
		if !errors.Is(acceptErr, net.ErrClosed) {
			t.Fatalf("Accept() error = %v, want error wrapping net.ErrClosed", acceptErr)
		}
	case <-time.After(transportTestTimeout):
		t.Fatal("Listener Close() did not unblock Accept()")
	}
}

// TestTcpDialerCanceledContext 验证已经取消的上下文会阻止拨号并返回 context.Canceled。
func TestTcpDialerCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	transport, err := NewTcpDialer(&TcpTransportConfig{}).Dial(ctx, "127.0.0.1:1")
	t.Logf("actual: transport=%v, err=%v", transport, err)
	t.Logf("expected: transport=<nil>, err wraps context.Canceled")

	if transport != nil {
		_ = transport.Close()
		t.Fatal("Dial() returned a transport for canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Dial() error = %v, want %v", err, context.Canceled)
	}
}

// TestTcpListenerRejectsInvalidAddress 验证监听地址无效时会明确返回错误。
func TestTcpListenerRejectsInvalidAddress(t *testing.T) {
	listener, err := NewTcpListener("invalid-address", &TcpTransportConfig{})
	t.Logf("actual: listener=%v, err=%v", listener, err)
	t.Logf("expected: listener=<nil>, err is non-nil")

	if listener != nil {
		_ = listener.Close()
		t.Fatal("NewTcpListener() returned a listener for invalid address")
	}
	if err == nil {
		t.Fatal("NewTcpListener() error = nil, want non-nil")
	}
}

// TestTcpTransportReadTimeoutRecovery 验证一次读取超时后 deadline 会被清除，后续读取仍可成功。
func TestTcpTransportReadTimeoutRecovery(t *testing.T) {
	transport, peer := newTcpTransportPair(t, &TcpTransportConfig{
		ReadBufferSize: 64,
		IO: &IOConfig{
			ReadTimeout: 30 * time.Millisecond,
		},
	})

	firstData, firstErr := transport.Read()
	if !errors.Is(firstErr, os.ErrDeadlineExceeded) {
		t.Fatalf("first Read() error = %v, want timeout", firstErr)
	}

	expected := []byte("data after timeout")
	writeResult := make(chan error, 1)
	go func() {
		_, err := peer.Write(expected)
		writeResult <- err
	}()

	actual, secondErr := transport.Read()
	t.Logf("actual: first_data=%q, first_err=%v, second_data=%q, second_err=%v", firstData, firstErr, actual, secondErr)
	t.Logf("expected: first_data=[], first_err wraps os.ErrDeadlineExceeded, second_data=%q, second_err=<nil>", expected)

	if secondErr != nil || !bytes.Equal(actual, expected) {
		t.Fatalf("second Read() = %q, %v", actual, secondErr)
	}
	select {
	case err := <-writeResult:
		if err != nil {
			t.Fatalf("peer Write() error = %v", err)
		}
	case <-time.After(transportTestTimeout):
		t.Fatal("peer Write() timed out")
	}
}

// TestTcpTransportWriteTimeoutRecovery 验证写入超时会返回明确错误，并在清除 deadline 后允许后续写入。
func TestTcpTransportWriteTimeoutRecovery(t *testing.T) {
	transport, peer := newTcpTransportPair(t, &TcpTransportConfig{
		IO: &IOConfig{
			WriteTimeout: 50 * time.Millisecond,
		},
	})
	if err := transport.conn.SetWriteBuffer(1024); err != nil {
		t.Fatalf("SetWriteBuffer() error = %v", err)
	}

	firstErr := transport.Write(make([]byte, 32<<20))
	if !errors.Is(firstErr, os.ErrDeadlineExceeded) {
		t.Fatalf("first Write() error = %v, want timeout", firstErr)
	}

	drainDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, peer)
		drainDone <- err
	}()

	secondErr := transport.Write([]byte("write after timeout"))
	t.Logf("actual: first_err=%v, second_err=%v", firstErr, secondErr)
	t.Logf("expected: first_err wraps os.ErrDeadlineExceeded, second_err=<nil>")
	if secondErr != nil {
		t.Fatalf("second Write() error = %v", secondErr)
	}

	if err := transport.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := peer.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		t.Fatalf("peer.Close() error = %v", err)
	}
	select {
	case <-drainDone:
	case <-time.After(transportTestTimeout):
		t.Fatal("peer drain did not stop")
	}
}

// TestTcpTransportConcurrentClose 验证多个 goroutine 同时关闭连接时结果一致且不存在竞态。
func TestTcpTransportConcurrentClose(t *testing.T) {
	transport, _ := newTcpTransportPair(t, &TcpTransportConfig{})
	const goroutines = 16

	var waitGroup sync.WaitGroup
	results := make(chan error, goroutines)
	waitGroup.Add(goroutines)
	for range goroutines {
		go func() {
			defer waitGroup.Done()
			results <- transport.Close()
		}()
	}
	waitGroup.Wait()
	close(results)

	actualErrors := make([]error, 0, goroutines)
	for err := range results {
		actualErrors = append(actualErrors, err)
		if err != nil {
			t.Fatalf("concurrent Close() error = %v", err)
		}
	}
	t.Logf("actual: close_calls=%d, errors=%v", len(actualErrors), actualErrors)
	t.Logf("expected: close_calls=%d, all errors=<nil>", goroutines)
}
