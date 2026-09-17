package network

import "errors"

var (
	// ErrClosed 表示资源已经完成关闭，不能再提供能力。
	ErrClosed = errors.New("network: closed")
	// ErrPacketTooLarge 表示出站编码结果超过限制
	ErrPacketTooLarge = errors.New("network: packet too large")
	// ErrMalformedPacket 表示解码后的包不合法
	ErrMalformedPacket = errors.New("network: malformed packet")

	// ErrConnectionExists 表示连接已经存在
	ErrConnectionExists = errors.New("network: connection already exists")
	// ErrAlreadyStarted 表示连接已经启动
	ErrAlreadyStarted = errors.New("network: already started")
)
