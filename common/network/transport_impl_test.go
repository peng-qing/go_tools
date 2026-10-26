package network

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// 使用环回地址和系统分配端口，不依赖外部服务。
func TestTransportRoundTrip(t *testing.T) {
	factories := map[string]func() (Listener, Dialer, error){
		"tcp": func() (Listener, Dialer, error) {
			conf := TCPConfig{ReadTimeout: testTimeout, WriteTimeout: testTimeout, NoDelay: true}
			ln, err := NewTCPListener("127.0.0.1:0", conf)
			return ln, NewTCPDialer(conf), err
		},
		"kcp": func() (Listener, Dialer, error) {
			conf := KCPConfig{ReadTimeout: testTimeout, WriteTimeout: testTimeout, NoDelay: true, IntervalMs: 10}
			ln, err := NewKCPListenerImpl("127.0.0.1:0", conf)
			return ln, NewKCPDialerImpl(conf), err
		},
		"ws": func() (Listener, Dialer, error) {
			conf := WsConfig{Path: "/stream", ReadTimeout: testTimeout, WriteTimeout: testTimeout}
			ln, err := NewWsListenerImpl(WsListenerConfig{Address: "127.0.0.1:0", WsConf: conf})
			return ln, NewWsDialerImpl(WsDialerConfig{WsConf: conf, Proxy: testNoProxy}), err
		},
	}
	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			listener, dialer, err := factory()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = listener.Close() })
			accepted := make(chan testAcceptResult, 1)
			go func() { conn, err := listener.Accept(); accepted <- testAcceptResult{transport: conn, err: err} }()
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
			defer cancel()
			client, err := dialer.DialContext(ctx, listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			// KCP 在收到首个数据包后才交付 Accept。
			if _, err := client.Write([]byte("hello")); err != nil {
				t.Fatal(err)
			}
			result := testReceive(t, accepted)
			if result.err != nil {
				t.Fatal(result.err)
			}
			server := result.transport
			t.Cleanup(func() { _ = server.Close() })
			data := make([]byte, 5)
			if _, err := io.ReadFull(server, data); err != nil || string(data) != "hello" {
				t.Fatalf("服务端读取: %q %v", data, err)
			}
			if _, err := server.Write([]byte("world")); err != nil {
				t.Fatal(err)
			}
			if _, err := io.ReadFull(client, data); err != nil || string(data) != "world" {
				t.Fatalf("客户端读取: %q %v", data, err)
			}
			if server.LocalAddr() == nil || server.RemoteAddr() == nil {
				t.Fatal("缺失连接地址")
			}
			if n, err := client.Read(nil); n != 0 || err != nil {
				t.Fatalf("空 Read: %d %v", n, err)
			}
			if n, err := client.Write(nil); n != 0 || err != nil {
				t.Fatalf("空 Write: %d %v", n, err)
			}
			readDone := make(chan error, 1)
			go func() { var b [1]byte; _, err := server.Read(b[:]); readDone <- err }()
			_ = server.Close()
			if err := testReceive(t, readDone); err == nil {
				t.Fatal("Close 未中断 Read")
			}
		})
	}
}

func testNoProxy(*http.Request) (*url.URL, error) { return nil, nil }

func TestWsEndpoint(t *testing.T) {
	tests := []struct {
		name, address, want string
		conf                WsDialerConfig
		invalid             bool
	}{
		{name: "default", address: "example.com:80", want: "ws://example.com:80/"},
		{name: "case_preserved", address: "WS://example.com/Case?token=AbC", want: "ws://example.com/Case?token=AbC"},
		{name: "query_url", address: "example.com/Path?next=https://example.org", want: "ws://example.com/Path?next=https://example.org"},
		{name: "configured_path", address: "example.com", want: "ws://example.com/Path", conf: WsDialerConfig{WsConf: WsConfig{Path: "Path"}}},
		{name: "default_tls", address: "example.com", want: "wss://example.com/", conf: WsDialerConfig{WsConf: WsConfig{TLSConfig: &tls.Config{}}}},
		{name: "explicit_plaintext", address: "ws://example.com", want: "ws://example.com/", conf: WsDialerConfig{WsConf: WsConfig{TLSConfig: &tls.Config{}}}},
		{name: "require_tls", address: "example.com", want: "wss://example.com/", conf: WsDialerConfig{RequireTLS: true}},
		{name: "reject_plaintext", address: "ws://example.com", conf: WsDialerConfig{RequireTLS: true}, invalid: true},
		{name: "empty", invalid: true},
		{name: "whitespace", address: " example.com", invalid: true},
		{name: "fragment", address: "ws://example.com/#", invalid: true},
		{name: "bad_scheme", address: "http://example.com", invalid: true},
		{name: "missing_host", address: "ws:///path", invalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewWsDialerImpl(tt.conf).endpoint(tt.address)
			if tt.invalid {
				if err == nil {
					t.Fatalf("应拒绝地址，得到 %q", got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %q %v want %q", got, err, tt.want)
			}
		})
	}
}

func TestWsPendingLimitAndClose(t *testing.T) {
	ln, err := NewWsListenerImpl(WsListenerConfig{Address: "127.0.0.1:0", MaxPendingConnections: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	dialer := websocket.Dialer{HandshakeTimeout: testTimeout, Proxy: testNoProxy}
	address := "ws://" + ln.Addr().String() + "/"
	first, _, err := dialer.Dial(address, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, response, err := dialer.Dial(address, nil)
	if second != nil {
		_ = second.Close()
	}
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("等待 Accept 的连接应占用 slot: response=%v err=%v", response, err)
	}
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	_ = first.SetReadDeadline(time.Now().Add(testTimeout))
	if _, _, err := first.ReadMessage(); err == nil {
		t.Fatal("未交付的连接没有关闭")
	}
	if _, err := ln.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("关闭后 Accept: %v", err)
	}
}

func TestWsBinaryStreamAndTextRejection(t *testing.T) {
	ln, err := NewWsListenerImpl(WsListenerConfig{Address: "127.0.0.1:0", WsConf: WsConfig{ReadTimeout: testTimeout}})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	dialer := websocket.Dialer{HandshakeTimeout: testTimeout, Proxy: testNoProxy}
	peer, _, err := dialer.Dial("ws://"+ln.Addr().String()+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	conn, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, data := range [][]byte{nil, []byte("ab"), []byte("cde")} {
		if err := peer.WriteMessage(websocket.BinaryMessage, data); err != nil {
			t.Fatal(err)
		}
	}
	data := make([]byte, 5)
	if _, err := io.ReadFull(conn, data); err != nil || string(data) != "abcde" {
		t.Fatalf("跨消息读取: %q %v", data, err)
	}
	if err := peer.WriteMessage(websocket.TextMessage, []byte("text")); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Read(data); err == nil {
		t.Fatal("应拒绝文本消息")
	}
}

func TestKCPInvalidConfiguration(t *testing.T) {
	for _, conf := range []KCPConfig{
		{Mode: KCPEncryption("unknown")},
		{Mode: KCPEncryptionAES, Key: []byte("short")},
		{DataShards: 200, ParityShards: 100},
	} {
		if ln, err := NewKCPListenerImpl("127.0.0.1:0", conf); err == nil {
			_ = ln.Close()
			t.Fatal("应拒绝无效配置")
		}
		if conn, err := NewKCPDialerImpl(conf).DialContext(context.Background(), "127.0.0.1:1"); err == nil {
			_ = conn.Close()
			t.Fatal("应拒绝无效配置")
		}
	}
}
