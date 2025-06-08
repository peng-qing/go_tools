package http_utils

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HttpHeader for request options
type HttpHeader struct {
	Params             map[string]string // URL 查询参数
	Data               map[string]string // 表单数据
	Headers            map[string]string // 自定义请求头
	Cookies            map[string]string // Cookie设置
	Auth               map[string]string // Basic Auth 认证
	JSON               map[string]any    // JSON 数据体
	Files              map[string]any    // 文件上传字段
	RowData            string            // 原始请求体
	Proxy              string            // 代理地址
	AllowRedirect      bool              // 是否允许重定向
	Timeout            int64             // 请求超时时间 单位-秒
	Chunked            bool              // 是否启用 chunked 模式（不设置 Content-Length）
	DisableKeepalives  bool              // 是否禁用 Keep-Alive
	DisableCompression bool              // 是否禁用压缩
	SkipVerifyTLS      bool              // 是否跳过 TLS 验证
}

// isConflict 检查冲突 Data/RowData/Files/Json 只能同时存在一个
func (h *HttpHeader) isConflict() bool {
	count := 0
	if len(h.Data) > 0 {
		count++
	}
	if len(h.RowData) > 0 {
		count++
	}
	if len(h.Files) > 0 {
		count++
	}
	if len(h.JSON) > 0 {
		count++
	}
	return count > 1
}

// applyReq 应用请求选项
func (h *HttpHeader) applyRequestOpt(r *http.Request) error {
	if h.isConflict() {
		return errors.New("Data/RowData/Files/Json 只能同时存在一个")
	}
	var err error
	if len(h.Params) > 0 {
		if err = setQuery(r, h.Params); err != nil {
			return err
		}
	}

	if len(h.Data) > 0 {
		if err = setData(r, h.Data, h.Chunked); err != nil {
			return err
		}
	}

	if len(h.JSON) > 0 {
		if err = setJSON(r, h.JSON, h.Chunked); err != nil {
			return err
		}
	}

	if len(h.Files) > 0 {
		if err = setFiles(r, h.Files, h.Chunked); err != nil {
			return err
		}
	}

	if len(h.RowData) > 0 {
		if err = setRowData(r, h.RowData, h.Chunked); err != nil {
			return err
		}
	}

	if len(h.Headers) > 0 {
		if err = setHeaders(r, h.Headers); err != nil {
			return err
		}
	}

	if len(h.Cookies) > 0 {
		if err = setCookies(r, h.Cookies); err != nil {
			return err
		}
	}

	if len(h.Auth) > 0 {
		if err = setAuth(r, h.Auth); err != nil {
			return err
		}
	}

	return nil
}

// applyClientOpt 应用客户端选项
func (h *HttpHeader) applyClientOpt(client *http.Client) error {
	if h.AllowRedirect {
		//  允许重定向 阻止跳转并返回最后一次的响应结果
		client.CheckRedirect = disableRedirect
	}
	//  设置超时时间
	client.Timeout = time.Duration(h.Timeout) * time.Second

	transport, ok := client.Transport.(*http.Transport)
	if ok {
		// 是否禁用 Keep-Alive
		transport.DisableKeepAlives = h.DisableKeepalives
		// 是否禁用压缩
		transport.DisableCompression = h.DisableCompression

		// 跳过 TLS 验证
		if h.SkipVerifyTLS {
			if transport.TLSClientConfig == nil {
				transport.TLSClientConfig = &tls.Config{}
			}
			transport.TLSClientConfig.InsecureSkipVerify = true
		}

		// 设置代理
		if h.Proxy != "" {
			proxyURL, err := url.Parse(h.Proxy)
			if err != nil {
				return err
			}
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return nil
}

// FileUploadMeta 文件上传数据
type FileUploadMeta struct {
	Data []byte // 文件内容
	Src  string // 文件路径
	Name string // 文件名
	MIME string // 文件类型 默认 application/octet-stream
}

// File 创建文件上传数据
func File(filename string, data []byte) *FileUploadMeta {
	return &FileUploadMeta{
		Data: data,
		Name: filename,
	}
}

// FileFromPath 创建文件上传数据
func FileFromPath(src string) *FileUploadMeta {
	return &FileUploadMeta{
		Src:  src,
		Name: filepath.Base(src),
	}
}

// SetSrc 设置文件路径
func (f *FileUploadMeta) SetSrc(src string) *FileUploadMeta {
	f.Src = src
	return f
}

// SetMIME 设置文件类型
func (f *FileUploadMeta) SetMIME(mime string) *FileUploadMeta {
	f.MIME = mime
	return f
}

var (
	escapeReplacer = strings.NewReplacer("\\", "\\\\", `"`, "\\\"")
)

// escapeSymbol 符号转义
func escapeSymbol(str string) string {
	return escapeReplacer.Replace(str)
}

// setQuery 设置查询参数
func setQuery(r *http.Request, params map[string]string) error {
	allQuery := make([]byte, 0)

	for key, value := range params {
		keyEscaped := url.QueryEscape(key)
		valEscaped := url.QueryEscape(value)

		allQuery = append(allQuery, '&')
		allQuery = append(allQuery, keyEscaped...)
		allQuery = append(allQuery, '=')
		allQuery = append(allQuery, valEscaped...)
	}

	// trim first &
	if r.URL.RawQuery == "" {
		allQuery = allQuery[1:]
	}

	r.URL.RawQuery = string(allQuery)

	return nil
}

// setData 设置表单数据
func setData(r *http.Request, allData map[string]string, chunked bool) error {
	data := ""
	for key, val := range allData {
		keyEscaped := url.QueryEscape(key)
		valEscaped := url.QueryEscape(val)

		data = fmt.Sprintf("%s&%s=%s", data, keyEscaped, valEscaped)
	}

	data = data[1:]
	reader := strings.NewReader(data)
	r.Body = io.NopCloser(reader)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if !chunked {
		r.ContentLength = reader.Size()
	}
	return nil
}

// setJSON 设置 JSON 数据体
func setJSON(r *http.Request, jsonData map[string]any, chunked bool) error {
	data, err := json.Marshal(jsonData)
	if err != nil {
		return err
	}
	reader := strings.NewReader(string(data))
	r.Body = io.NopCloser(reader)
	r.Header.Set("Content-Type", "application/json")
	if !chunked {
		r.ContentLength = reader.Size()
	}
	return nil
}

// setRowData 设置原始请求体
func setRowData(r *http.Request, rowData string, chunked bool) error {
	reader := strings.NewReader(rowData)
	r.Body = io.NopCloser(reader)
	if !chunked {
		r.ContentLength = reader.Size()
	}
	return nil
}

// setFiles 添加文件上传字段
func setFiles(r *http.Request, files map[string]any, chunked bool) error {
	buffer := bytes.NewBuffer(make([]byte, 0))
	writer := multipart.NewWriter(buffer)

	for key, val := range files {
		switch val := val.(type) {
		case *FileUploadMeta:
			mimeType := val.MIME
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}

			header := make(textproto.MIMEHeader)
			header.Set("Content-Disposition",
				fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeSymbol(key), escapeSymbol(val.Name)))
			header.Set("Content-Type", mimeType)

			part, err := writer.CreatePart(header)
			if err != nil {
				return err
			}
			if err = processFileUploadMeta(part, val); err != nil {
				return err
			}
		case string:
			err := writer.WriteField(key, val)
			if err != nil {
				return err
			}
		default:
			return errors.New("unsupported file upload type")
		}
	}

	err := writer.Close()
	if err != nil {
		return err
	}

	r.Body = io.NopCloser(buffer)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	if !chunked {
		r.ContentLength = int64(buffer.Len())
	}
	return nil
}

// processFileUploadMeta 处理文件上传元数据 辅助函数
func processFileUploadMeta(writer io.Writer, val *FileUploadMeta) error {
	var err error
	if len(val.Data) > 0 {
		_, err = writer.Write(val.Data)
		if err != nil {
			return err
		}
		return nil
	}
	file, err := os.Open(val.Src)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(writer, file)
	if err != nil {
		return err
	}
	return nil
}

// setHeaders 设置请求头
func setHeaders(r *http.Request, headers map[string]string) error {
	for headerKey, headerVal := range headers {
		r.Header.Set(headerKey, headerVal)
	}
	return nil
}

// setCookies 设置 Cookie
func setCookies(r *http.Request, cookies map[string]string) error {
	for cookieKey, cookieVal := range cookies {
		cookie := http.Cookie{
			Name:  cookieKey,
			Value: cookieVal,
		}
		r.AddCookie(&cookie)
	}
	return nil
}

// setAuth 设置认证信息
func setAuth(r *http.Request, auth map[string]string) error {
	for authKey, authVal := range auth {
		r.SetBasicAuth(authKey, authVal)
	}
	return nil
}
