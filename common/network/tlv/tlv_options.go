package tlv

import (
	"time"

	"go_tools/common/options"
)

// WithTcpConnReadTimeout 设置连接读超时Options
func WithTcpConnReadTimeout(readTimeout int64) options.Option[TLVTCPConnection] {
	return options.WrapperOptions[TLVTCPConnection](func(t *TLVTCPConnection) {
		t.readTimeout = time.Duration(readTimeout)
	})
}

// WithTcpConnWriteTimeout 设置连接写超时Options
func WithTcpConnWriteTimeout(writeTimeout int64) options.Option[TLVTCPConnection] {
	return options.WrapperOptions[TLVTCPConnection](func(t *TLVTCPConnection) {
		t.writeTimeout = time.Duration(writeTimeout)
	})
}
