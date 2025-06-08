package http_utils

import (
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
)

var (
	// userAgent
	userAgent = "go_tools/http_utils"

	// defaultCheckRedirect 默认请求重定向策略
	defaultCheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 10 {
			return errors.New("")
		}
		return nil
	}

	// disableRedirect 禁用重定向
	disableRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	// supportHttpMethod 支持的http方法
	supportHttpMethod = map[string]bool{
		http.MethodGet:     true,
		http.MethodPost:    true,
		http.MethodDelete:  true,
		http.MethodPut:     true,
		http.MethodPatch:   true,
		http.MethodHead:    true,
		http.MethodOptions: true,
	}
)

// RequestHookFunc request hook
type RequestHookFunc func(r *http.Request) error

// ResponseHookFunc response hook
type ResponseHookFunc func(r *http.Response) error

// Session http session
type Session struct {
	Client             *http.Client
	beforeRequestHooks []RequestHookFunc
	afterResponseHooks []ResponseHookFunc
	lock               sync.Mutex
}

func newHttpClient() *http.Client {
	client := &http.Client{}
	jar, _ := cookiejar.New(nil)
	client.Jar = jar
	client.Transport = &http.Transport{}
	return client
}

// NewSession new session
func NewSession() *Session {
	return &Session{
		Client: newHttpClient(),
	}
}

func (s *Session) Request(method string, urlStr string, options *HttpHeader) (*http.Request, *Response, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// 创建客户端
	if s.Client == nil {
		s.Client = newHttpClient()
	}
	var (
		req *http.Request
		err error
	)

	method = strings.ToUpper(method)
	// 检查方法
	if !supportHttpMethod[method] {
		return nil, nil, errors.New("unsupported method")
	}

	// 解析URL
	finalURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, nil, err
	}
	finalURL.RawQuery = finalURL.Query().Encode() // 添加参数
	// 创建请求
	req, err = http.NewRequest(method, finalURL.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", userAgent) // 设置UA
	req.Close = true

	// 设置请求头
	if options != nil {
		if err = options.applyRequestOpt(req); err != nil {
			return nil, nil, err
		}
		// 提前注册恢复Client默认配置状态 确保每次请求独立
		currentCheckRedirect := s.Client.CheckRedirect
		currentTimeout := s.Client.Timeout
		currentTransport := s.Client.Transport
		defer func() {
			s.Client.CheckRedirect = currentCheckRedirect
			s.Client.Timeout = currentTimeout
			s.Client.Transport = currentTransport
		}()
		// 应用客户端选项
		if err = options.applyClientOpt(s.Client); err != nil {
			return nil, nil, err
		}
	}

	// 执行钩子函数
	for _, hookFn := range s.beforeRequestHooks {
		if err = hookFn(req); err != nil {
			return nil, nil, err
		}
	}

	// 发送请求
	rowResp, err := s.Client.Do(req)
	if err != nil {
		return nil, nil, err
	}

	// 执行钩子函数
	for _, hookFn := range s.afterResponseHooks {
		if err = hookFn(rowResp); err != nil {
			return nil, nil, err
		}
	}

	resp, err := NewResponse(rowResp)
	return req, resp, err
}

// AddRequestHooks 添加请求钩子函数
func (s *Session) AddRequestHooks(hooks ...RequestHookFunc) error {
	if len(hooks) <= 0 {
		return errors.New("request hook function is nil")
	}
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.beforeRequestHooks == nil {
		s.beforeRequestHooks = make([]RequestHookFunc, 0)
	}

	s.beforeRequestHooks = append(s.beforeRequestHooks, hooks...)
	return nil
}

// ResetRequestHooks 重置请求钩子函数
func (s *Session) ResetRequestHooks() {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.beforeRequestHooks = make([]RequestHookFunc, 0)
}

// AddResponseHooks 添加响应钩子函数
func (s *Session) AddResponseHooks(hooks ...ResponseHookFunc) error {
	if len(hooks) <= 0 {
		return errors.New("response hook function is nil")
	}
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.afterResponseHooks == nil {
		s.afterResponseHooks = make([]ResponseHookFunc, 0)
	}
	s.afterResponseHooks = append(s.afterResponseHooks, hooks...)

	return nil
}

// ResetResponseHooks 重置响应钩子函数
func (s *Session) ResetResponseHooks() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.afterResponseHooks = make([]ResponseHookFunc, 0)
}

// Get 使用get请求
func (s *Session) Get(urlStr string, options *HttpHeader) (*http.Request, *Response, error) {
	return s.Request(http.MethodGet, urlStr, options)
}

// Post 使用post请求
func (s *Session) Post(urlStr string, options *HttpHeader) (*http.Request, *Response, error) {
	return s.Request(http.MethodPost, urlStr, options)
}

// Delete 使用delete请求
func (s *Session) Delete(urlStr string, options *HttpHeader) (*http.Request, *Response, error) {
	return s.Request(http.MethodDelete, urlStr, options)
}

// Put 使用put请求
func (s *Session) Put(urlStr string, options *HttpHeader) (*http.Request, *Response, error) {
	return s.Request(http.MethodPut, urlStr, options)
}

// Patch 使用patch请求
func (s *Session) Patch(urlStr string, options *HttpHeader) (*http.Request, *Response, error) {
	return s.Request(http.MethodPatch, urlStr, options)
}

// Head 使用head请求
func (s *Session) Head(urlStr string, options *HttpHeader) (*http.Request, *Response, error) {
	return s.Request(http.MethodHead, urlStr, options)
}

// Options 使用options请求
func (s *Session) Options(urlStr string, options *HttpHeader) (*http.Request, *Response, error) {
	return s.Request(http.MethodOptions, urlStr, options)
}
