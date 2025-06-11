package http_utils

import (
	"net/http"
	"sync"
)

var (
	once    = sync.Once{}
	session *Session
)

func init() {
	once.Do(func() {
		session = NewSession()
	})
}

// Get 请求
func Get(url string, options *HttpHeader) (*http.Request, *Response, error) {
	return session.Get(url, options)
}

// Post 请求
func Post(url string, options *HttpHeader) (*http.Request, *Response, error) {
	return session.Post(url, options)
}

// Delete 删除请求
func Delete(url string, options *HttpHeader) (*http.Request, *Response, error) {
	return session.Delete(url, options)
}

// Put 请求
func Put(url string, options *HttpHeader) (*http.Request, *Response, error) {
	return session.Put(url, options)
}

// Patch 请求
func Patch(url string, options *HttpHeader) (*http.Request, *Response, error) {
	return session.Patch(url, options)
}

// Head 请求
func Head(url string, options *HttpHeader) (*http.Request, *Response, error) {
	return session.Head(url, options)
}

// Options 获取请求
func Options(url string, options *HttpHeader) (*http.Request, *Response, error) {
	return session.Options(url, options)
}
