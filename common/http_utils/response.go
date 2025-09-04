package http_utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/peng-qing/go_tools/common/encode_utils"
)

var (
	ErrUnrecognizedEncoding = errors.New("unrecognized encoding string")
)

// Response 响应对象 wrapper for http.Response
type Response struct {
	*http.Response
	encoding string
	Text     string
	Bytes    []byte
}

// NewResponse 创建一个响应对象
func NewResponse(r *http.Response) (*Response, error) {
	resp := &Response{
		Response: r,
		encoding: encode_utils.EncodingUTF8,
		Text:     "",
		Bytes:    make([]byte, 0),
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	r.Body = io.NopCloser(bytes.NewBuffer(data))
	resp.Bytes = data
	resp.Text = string(data)

	return resp, nil
}

// JSON 解析响应数据为 JSON
// 这里不一定是 application/json 因为是直接对body数据进行解析
func (r *Response) JSON(v any) error {
	return json.Unmarshal(r.Bytes, v)
}

// SetEncoding 设置响应编码
func (r *Response) SetEncoding(e string) error {
	e = strings.ToUpper(e)
	if e != r.encoding {
		r.encoding = e
		encoder := encode_utils.NewEncoder(e)
		if encoder == nil {
			return ErrUnrecognizedEncoding
		}
		text, err := encoder.String(r.Text)
		if err != nil {
			return err
		}
		r.Text = text
	}

	return nil
}

// GetEncoding 获取响应编码
func (r *Response) GetEncoding() string {
	return r.encoding
}

// SaveFile 保存响应数据到文件
func (r *Response) SaveFile(filename string) error {
	dst, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = dst.Write(r.Bytes)
	return err
}
