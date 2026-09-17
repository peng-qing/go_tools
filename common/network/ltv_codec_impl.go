package network

import (
	"encoding/binary"
	"slices"
)

var (
	_ Codec[*LtvPacket] = (*LtvCodecImpl)(nil)
)

const (
	// LtvHeaderSize 是LTV协议的包头大小 length(4)+type(4)
	LtvHeaderSize = 8
)

// LtvPacket 是LTV协议的包结构
// | length | type | payload |
// | 4      | 4    | variable |
// 注: length 指的是 payload 的长度
type LtvPacket struct {
	Type    uint32
	Payload []byte
}

// NewLtvPacket 创建一个LTV协议的包
func NewLtvPacket(typ uint32, payload []byte) *LtvPacket {
	return &LtvPacket{
		Type:    typ,
		Payload: slices.Clone(payload),
	}
}

// LtvCodecImpl 是LTV协议的编解码器实现
type LtvCodecImpl struct {
	maxPayload int
	order      binary.ByteOrder
}

// NewLtvCodec 创建一个LTV协议的编解码器
func NewLtvCodec(maxPayload int, littleEndian bool) *LtvCodecImpl {
	order := binary.ByteOrder(binary.BigEndian)
	if littleEndian {
		order = binary.LittleEndian
	}
	return &LtvCodecImpl{
		maxPayload: maxPayload,
		order:      order,
	}
}

// Decode 解码LTV协议的包
func (c *LtvCodecImpl) Decode(data []byte) (packet *LtvPacket, consumed int, err error) {
	if len(data) < LtvHeaderSize {
		// 数据不足包头 等待后续数据
		return nil, 0, nil
	}
	dataLen := c.order.Uint32(data[0:4])
	if dataLen > uint32(c.maxPayload) {
		return nil, 0, ErrPacketTooLarge
	}
	totalLen := int(dataLen) + LtvHeaderSize
	if totalLen > len(data) {
		// 数据不足解析一个完整包 等待后续数据
		return nil, 0, nil
	}
	msgType := c.order.Uint32(data[4:8])
	payload := slices.Clone(data[LtvHeaderSize:int(totalLen)])

	return &LtvPacket{Type: msgType, Payload: payload}, totalLen, nil
}

// Encode 编码LTV协议的包
func (c *LtvCodecImpl) Encode(packet *LtvPacket) ([]byte, error) {
	if len(packet.Payload) > c.maxPayload {
		return nil, ErrPacketTooLarge
	}
	totalLen := len(packet.Payload) + LtvHeaderSize
	encoded := make([]byte, totalLen)
	c.order.PutUint32(encoded[0:4], uint32(len(packet.Payload)))
	c.order.PutUint32(encoded[4:8], packet.Type)
	copy(encoded[8:], packet.Payload)
	return encoded, nil
}
