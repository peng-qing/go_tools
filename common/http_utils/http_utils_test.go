package http_utils

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/peng-qing/go_tools/common/encode_utils"
	"golang.org/x/text/encoding/simplifiedchinese"
)

type echoPayload struct {
	Method        string              `json:"method"`
	Path          string              `json:"path"`
	Query         url.Values          `json:"query"`
	Headers       http.Header         `json:"headers"`
	Cookies       map[string]string   `json:"cookies"`
	Body          string              `json:"body"`
	ContentLength int64               `json:"contentLength"`
	ContentType   string              `json:"contentType"`
	Username      string              `json:"username"`
	Password      string              `json:"password"`
	UserAgent     string              `json:"userAgent"`
}

func newEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		cookies := make(map[string]string)
		for _, cookie := range r.Cookies() {
			cookies[cookie.Name] = cookie.Value
		}
		username, password, _ := r.BasicAuth()
		_ = json.NewEncoder(w).Encode(echoPayload{
			Method:        r.Method,
			Path:          r.URL.Path,
			Query:         r.URL.Query(),
			Headers:       r.Header,
			Cookies:       cookies,
			Body:          string(body),
			ContentLength: r.ContentLength,
			ContentType:   r.Header.Get("Content-Type"),
			Username:      username,
			Password:      password,
			UserAgent:     r.UserAgent(),
		})
	}))
	t.Cleanup(server.Close)
	return server
}

func decodeEcho(t *testing.T, resp *Response) echoPayload {
	t.Helper()
	var payload echoPayload
	if err := resp.JSON(&payload); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	return payload
}

func mustRequest(t *testing.T, session *Session, method, urlStr string, options *HttpHeader) (*http.Request, *Response) {
	t.Helper()
	req, resp, err := session.Request(method, urlStr, options)
	if err != nil {
		t.Fatalf("Request(%s) error = %v", method, err)
	}
	if req == nil || resp == nil {
		t.Fatalf("Request(%s) returned nil request or response", method)
	}
	return req, resp
}

func newHTTPResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestNewResponse(t *testing.T) {
	t.Run("nil response", func(t *testing.T) {
		resp, err := NewResponse(nil)
		if err == nil || resp != nil {
			t.Fatalf("NewResponse(nil) = (%v, %v), want error", resp, err)
		}
	})

	t.Run("nil body", func(t *testing.T) {
		resp, err := NewResponse(&http.Response{})
		if err == nil || resp != nil {
			t.Fatalf("NewResponse(nil body) = (%v, %v), want error", resp, err)
		}
	})

	t.Run("reads body and restores it", func(t *testing.T) {
		raw := newHTTPResponse(`{"ok":true}`)
		resp, err := NewResponse(raw)
		if err != nil {
			t.Fatalf("NewResponse() error = %v", err)
		}
		if got, want := resp.Text, `{"ok":true}`; got != want {
			t.Fatalf("Text = %q, want %q", got, want)
		}
		if got, want := string(resp.Bytes), `{"ok":true}`; got != want {
			t.Fatalf("Bytes = %q, want %q", got, want)
		}
		if got, want := resp.GetEncoding(), encode_utils.EncodingUTF8; got != want {
			t.Fatalf("GetEncoding() = %q, want %q", got, want)
		}

		replayed, err := io.ReadAll(raw.Body)
		if err != nil {
			t.Fatalf("read restored body: %v", err)
		}
		if got, want := string(replayed), `{"ok":true}`; got != want {
			t.Fatalf("restored body = %q, want %q", got, want)
		}
	})
}

func TestResponseJSON(t *testing.T) {
	resp, err := NewResponse(newHTTPResponse(`{"name":"go","count":2}`))
	if err != nil {
		t.Fatalf("NewResponse() error = %v", err)
	}

	var data struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}
	if err := resp.JSON(&data); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}
	if data.Name != "go" || data.Count != 2 {
		t.Fatalf("JSON() = %+v, want name=go count=2", data)
	}

	if err := resp.JSON(struct{}{}); err == nil {
		t.Fatal("JSON(non-pointer) did not return error")
	}
}

func TestResponseEncoding(t *testing.T) {
	gbkBytes, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("你好"))
	if err != nil {
		t.Fatalf("encode GBK: %v", err)
	}

	resp, err := NewResponse(&http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(string(gbkBytes))),
		Header:     make(http.Header),
	})
	if err != nil {
		t.Fatalf("NewResponse() error = %v", err)
	}

	if err := resp.SetEncoding("gbk"); err != nil {
		t.Fatalf("SetEncoding(GBK) error = %v", err)
	}
	if got := resp.Text; got != "你好" {
		t.Fatalf("Text after GBK decode = %q, want 你好", got)
	}
	if got, want := resp.GetEncoding(), encode_utils.EncodingGBK; got != want {
		t.Fatalf("GetEncoding() = %q, want %q", got, want)
	}

	if err := resp.SetEncoding(encode_utils.EncodingGBK); err != nil {
		t.Fatalf("SetEncoding same encoding error = %v", err)
	}

	if err := resp.SetEncoding("unknown"); !errors.Is(err, ErrUnrecognizedEncoding) {
		t.Fatalf("SetEncoding(unknown) error = %v, want %v", err, ErrUnrecognizedEncoding)
	}
}

func TestResponseSaveFile(t *testing.T) {
	resp, err := NewResponse(newHTTPResponse("saved-content"))
	if err != nil {
		t.Fatalf("NewResponse() error = %v", err)
	}

	path := filepath.Join(t.TempDir(), "resp.txt")
	if err := resp.SaveFile(path); err != nil {
		t.Fatalf("SaveFile() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(data), "saved-content"; got != want {
		t.Fatalf("file content = %q, want %q", got, want)
	}

	if err := resp.SaveFile(filepath.Join(t.TempDir(), "missing", "resp.txt")); err == nil {
		t.Fatal("SaveFile() to missing directory did not return error")
	}
}

func TestSessionUnsupportedMethodAndInvalidURL(t *testing.T) {
	session := NewSession()

	_, _, err := session.Request("TRACE", "http://example.com", nil)
	if err == nil {
		t.Fatal("unsupported method did not return error")
	}

	_, _, err = session.Request(http.MethodGet, "://bad-url", nil)
	if err == nil {
		t.Fatal("invalid url did not return error")
	}
}

func TestSessionNilClientIsInitialized(t *testing.T) {
	server := newEchoServer(t)
	session := &Session{}

	_, resp := mustRequest(t, session, http.MethodGet, server.URL+"/init", nil)
	payload := decodeEcho(t, resp)
	if payload.Method != http.MethodGet {
		t.Fatalf("method = %q, want GET", payload.Method)
	}
	if session.Client == nil {
		t.Fatal("Client was not initialized")
	}
}

func TestSessionHTTPMethods(t *testing.T) {
	server := newEchoServer(t)
	session := NewSession()

	methods := []struct {
		name string
		fn   func(string, *HttpHeader) (*http.Request, *Response, error)
		want string
	}{
		{"Get", session.Get, http.MethodGet},
		{"Post", session.Post, http.MethodPost},
		{"Delete", session.Delete, http.MethodDelete},
		{"Put", session.Put, http.MethodPut},
		{"Patch", session.Patch, http.MethodPatch},
		{"Head", session.Head, http.MethodHead},
		{"Options", session.Options, http.MethodOptions},
	}

	for _, method := range methods {
		t.Run(method.name, func(t *testing.T) {
			_, resp, err := method.fn(server.URL+"/method", nil)
			if err != nil {
				t.Fatalf("%s() error = %v", method.name, err)
			}
			if method.want == http.MethodHead {
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
				}
				return
			}
			payload := decodeEcho(t, resp)
			if payload.Method != method.want {
				t.Fatalf("method = %q, want %q", payload.Method, method.want)
			}
		})
	}

	_, resp := mustRequest(t, session, "get", server.URL+"/lower", nil)
	payload := decodeEcho(t, resp)
	if payload.Method != http.MethodGet {
		t.Fatalf("lowercase method = %q, want GET", payload.Method)
	}
	if payload.UserAgent != userAgent {
		t.Fatalf("User-Agent = %q, want %q", payload.UserAgent, userAgent)
	}
}

func TestSessionRequestOptions(t *testing.T) {
	server := newEchoServer(t)
	session := NewSession()

	t.Run("params headers cookies auth", func(t *testing.T) {
		_, resp := mustRequest(t, session, http.MethodGet, server.URL+"/query?existed=1", &HttpHeader{
			Params:  map[string]string{"name": "go", "existed": "2"},
			Headers: map[string]string{"X-Test": "header-value"},
			Cookies: map[string]string{"token": "abc"},
			Auth:    map[string]string{"alice": "secret"},
		})
		payload := decodeEcho(t, resp)
		if got := payload.Query.Get("name"); got != "go" {
			t.Fatalf("query name = %q, want go", got)
		}
		if got := payload.Query.Get("existed"); got != "2" {
			t.Fatalf("query existed = %q, want 2", got)
		}
		if got := payload.Headers.Get("X-Test"); got != "header-value" {
			t.Fatalf("X-Test = %q, want header-value", got)
		}
		if got := payload.Cookies["token"]; got != "abc" {
			t.Fatalf("cookie token = %q, want abc", got)
		}
		if payload.Username != "alice" || payload.Password != "secret" {
			t.Fatalf("basic auth = %s:%s, want alice:secret", payload.Username, payload.Password)
		}
	})

	t.Run("form data", func(t *testing.T) {
		_, resp := mustRequest(t, session, http.MethodPost, server.URL+"/form", &HttpHeader{
			Data: map[string]string{"k": "v"},
		})
		payload := decodeEcho(t, resp)
		if payload.ContentType != "application/x-www-form-urlencoded" {
			t.Fatalf("Content-Type = %q", payload.ContentType)
		}
		if payload.Body != "k=v" {
			t.Fatalf("body = %q, want k=v", payload.Body)
		}
		if payload.ContentLength != int64(len("k=v")) {
			t.Fatalf("ContentLength = %d, want %d", payload.ContentLength, len("k=v"))
		}
	})

	t.Run("json body", func(t *testing.T) {
		_, resp := mustRequest(t, session, http.MethodPost, server.URL+"/json", &HttpHeader{
			JSON: map[string]any{"id": 1, "ok": true},
		})
		payload := decodeEcho(t, resp)
		if payload.ContentType != "application/json" {
			t.Fatalf("Content-Type = %q", payload.ContentType)
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(payload.Body), &body); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if body["id"].(float64) != 1 || body["ok"] != true {
			t.Fatalf("json body = %s", payload.Body)
		}
	})

	t.Run("raw data chunked", func(t *testing.T) {
		_, resp := mustRequest(t, session, http.MethodPost, server.URL+"/raw", &HttpHeader{
			RowData: "raw-body",
			Chunked: true,
		})
		payload := decodeEcho(t, resp)
		if payload.Body != "raw-body" {
			t.Fatalf("body = %q, want raw-body", payload.Body)
		}
		if payload.ContentLength != -1 {
			t.Fatalf("chunked ContentLength = %d, want -1", payload.ContentLength)
		}
	})

	t.Run("body conflict", func(t *testing.T) {
		_, _, err := session.Post(server.URL+"/conflict", &HttpHeader{
			Data:    map[string]string{"a": "1"},
			RowData: "raw",
		})
		if err == nil {
			t.Fatal("conflicting body fields did not return error")
		}
	})
}

func TestSessionFileUpload(t *testing.T) {
	var receivedName, receivedMIME, receivedBody, receivedNote string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		receivedNote = r.FormValue("note")
		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()
		receivedName = header.Filename
		receivedMIME = header.Header.Get("Content-Type")
		data, _ := io.ReadAll(file)
		receivedBody = string(data)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	session := NewSession()
	_, _, err := session.Post(server.URL, &HttpHeader{
		Files: map[string]any{
			"file": File(`a"b.txt`, []byte("hello")).SetMIME("text/plain"),
			"note": "from-form",
		},
	})
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	if receivedName != `a\"b.txt` && receivedName != `a"b.txt` {
		t.Fatalf("filename = %q, want escaped or original name", receivedName)
	}
	if receivedMIME != "text/plain" {
		t.Fatalf("mime = %q, want text/plain", receivedMIME)
	}
	if receivedBody != "hello" {
		t.Fatalf("file body = %q, want hello", receivedBody)
	}
	if receivedNote != "from-form" {
		t.Fatalf("note = %q, want from-form", receivedNote)
	}

	src := filepath.Join(t.TempDir(), "from-path.txt")
	if err := os.WriteFile(src, []byte("from-path"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, _, err = session.Post(server.URL, &HttpHeader{
		Files: map[string]any{
			"file": FileFromPath(src).SetSrc(src),
		},
	})
	if err != nil {
		t.Fatalf("Post FileFromPath error = %v", err)
	}
	if receivedName != "from-path.txt" {
		t.Fatalf("filename = %q, want from-path.txt", receivedName)
	}
	if receivedBody != "from-path" {
		t.Fatalf("file body = %q, want from-path", receivedBody)
	}

	_, _, err = session.Post(server.URL, &HttpHeader{
		Files: map[string]any{"file": 123},
	})
	if err == nil {
		t.Fatal("unsupported file upload type did not return error")
	}

	_, _, err = session.Post(server.URL, &HttpHeader{
		Files: map[string]any{"file": FileFromPath(filepath.Join(t.TempDir(), "missing.txt"))},
	})
	if err == nil {
		t.Fatal("missing upload file did not return error")
	}
}

func TestSessionRedirectAndClientRestore(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new", http.StatusFound)
	})
	mux.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("arrived"))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	session := NewSession()
	originalTimeout := session.Client.Timeout
	originalTransport := session.Client.Transport

	_, resp, err := session.Get(server.URL+"/old", &HttpHeader{AllowRedirect: false})
	if err != nil {
		t.Fatalf("Get without redirect error = %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusFound)
	}

	_, resp, err = session.Get(server.URL+"/old", &HttpHeader{AllowRedirect: true, Timeout: 5})
	if err != nil {
		t.Fatalf("Get with redirect error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if resp.Text != "arrived" {
		t.Fatalf("body = %q, want arrived", resp.Text)
	}
	if session.Client.Timeout != originalTimeout {
		t.Fatalf("Timeout was not restored: %v", session.Client.Timeout)
	}
	if session.Client.Transport != originalTransport {
		t.Fatal("Transport was not restored")
	}
}

func TestSessionHooks(t *testing.T) {
	server := newEchoServer(t)
	session := NewSession()

	if err := session.AddRequestHooks(); err == nil {
		t.Fatal("AddRequestHooks() with no hooks did not return error")
	}
	if err := session.AddResponseHooks(); err == nil {
		t.Fatal("AddResponseHooks() with no hooks did not return error")
	}

	var requestCalled, responseCalled atomic.Int32
	if err := session.AddRequestHooks(func(r *http.Request) error {
		requestCalled.Add(1)
		r.Header.Set("X-Hook", "1")
		return nil
	}); err != nil {
		t.Fatalf("AddRequestHooks() error = %v", err)
	}
	if err := session.AddResponseHooks(func(r *http.Response) error {
		responseCalled.Add(1)
		return nil
	}); err != nil {
		t.Fatalf("AddResponseHooks() error = %v", err)
	}

	_, resp := mustRequest(t, session, http.MethodGet, server.URL+"/hooks", nil)
	payload := decodeEcho(t, resp)
	if payload.Headers.Get("X-Hook") != "1" {
		t.Fatal("request hook did not set header")
	}
	if requestCalled.Load() != 1 || responseCalled.Load() != 1 {
		t.Fatalf("hooks called request=%d response=%d, want 1/1", requestCalled.Load(), responseCalled.Load())
	}

	session.ResetRequestHooks()
	session.ResetResponseHooks()
	requestCalled.Store(0)
	responseCalled.Store(0)
	_, _ = mustRequest(t, session, http.MethodGet, server.URL+"/hooks-reset", nil)
	if requestCalled.Load() != 0 || responseCalled.Load() != 0 {
		t.Fatal("reset hooks were still invoked")
	}

	_ = session.AddRequestHooks(func(*http.Request) error {
		return errors.New("request hook failed")
	})
	if _, _, err := session.Get(server.URL+"/hook-err", nil); err == nil {
		t.Fatal("request hook error was not returned")
	}
	session.ResetRequestHooks()

	_ = session.AddResponseHooks(func(*http.Response) error {
		return errors.New("response hook failed")
	})
	if _, _, err := session.Get(server.URL+"/hook-err", nil); err == nil {
		t.Fatal("response hook error was not returned")
	}
}

func TestApplyClientOpt(t *testing.T) {
	client := newHttpClient()
	header := &HttpHeader{
		AllowRedirect:      false,
		Timeout:            3,
		DisableKeepalives:  true,
		DisableCompression: true,
		SkipVerifyTLS:      true,
		Proxy:              "http://127.0.0.1:8080",
	}
	if err := header.applyClientOpt(client); err != nil {
		t.Fatalf("applyClientOpt() error = %v", err)
	}
	if client.Timeout != 3*time.Second {
		t.Fatalf("Timeout = %v, want 3s", client.Timeout)
	}
	if client.CheckRedirect == nil {
		t.Fatal("CheckRedirect was not set")
	}
	if err := client.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("CheckRedirect error = %v, want ErrUseLastResponse", err)
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}
	if !transport.DisableKeepAlives || !transport.DisableCompression {
		t.Fatal("keepalive/compression flags were not applied")
	}
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("SkipVerifyTLS was not applied")
	}
	if transport.Proxy == nil {
		t.Fatal("Proxy was not applied")
	}

	bad := &HttpHeader{Proxy: "http://[::1"}
	if err := bad.applyClientOpt(newHttpClient()); err == nil {
		t.Fatal("invalid proxy did not return error")
	}
}

func TestFileHelpersAndEscapeSymbol(t *testing.T) {
	meta := File("name.txt", []byte("data")).SetMIME("text/plain")
	if meta.Name != "name.txt" || meta.MIME != "text/plain" || string(meta.Data) != "data" {
		t.Fatalf("File() = %+v", meta)
	}

	fromPath := FileFromPath("/tmp/a.txt").SetSrc("/tmp/b.txt")
	if fromPath.Name != "a.txt" || fromPath.Src != "/tmp/b.txt" {
		t.Fatalf("FileFromPath() = %+v", fromPath)
	}

	if got, want := escapeSymbol(`a\b"c`), `a\\b\"c`; got != want {
		t.Fatalf("escapeSymbol() = %q, want %q", got, want)
	}
}

func TestWrapperMethods(t *testing.T) {
	server := newEchoServer(t)

	wrappers := []struct {
		name string
		fn   func(string, *HttpHeader) (*http.Request, *Response, error)
		want string
	}{
		{"Get", Get, http.MethodGet},
		{"Post", Post, http.MethodPost},
		{"Delete", Delete, http.MethodDelete},
		{"Put", Put, http.MethodPut},
		{"Patch", Patch, http.MethodPatch},
		{"Head", Head, http.MethodHead},
		{"Options", Options, http.MethodOptions},
	}

	for _, wrapper := range wrappers {
		t.Run(wrapper.name, func(t *testing.T) {
			_, resp, err := wrapper.fn(server.URL+"/wrapper", nil)
			if err != nil {
				t.Fatalf("%s() error = %v", wrapper.name, err)
			}
			if wrapper.want == http.MethodHead {
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
				}
				return
			}
			payload := decodeEcho(t, resp)
			if payload.Method != wrapper.want {
				t.Fatalf("method = %q, want %q", payload.Method, wrapper.want)
			}
		})
	}
}
