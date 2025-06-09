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

func Get(url string, options *HttpHeader) (*http.Request, *Response, error) {
	return session.Get(url, options)
}

func Post(url string, options *HttpHeader) (*http.Request, *Response, error) {
	return session.Post(url, options)
}

func Delete(url string, options *HttpHeader) (*http.Request, *Response, error) {
	return session.Delete(url, options)
}
func Put(url string, options *HttpHeader) (*http.Request, *Response, error) {
	return session.Put(url, options)
}
func Patch(url string, options *HttpHeader) (*http.Request, *Response, error) {
	return session.Patch(url, options)
}
func Head(url string, options *HttpHeader) (*http.Request, *Response, error) {
	return session.Head(url, options)
}
func Options(url string, options *HttpHeader) (*http.Request, *Response, error) {
	return session.Options(url, options)
}
