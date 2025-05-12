package ltv

import (
	"bytes"
	"encoding/binary"

	"go_tools/common/network"
)

const (
	// LTVHeaderSize ltv格式包头大小
	// |  length(4byte)  |  type(4byte)  |
	LTVHeaderSize = 8
)

var (
	// LTVProtocolCoder 实现了IProtocolCoder接口
	_ network.IProtocolCoder = (*LTVProtocolCoder)(nil)
)

// LTVProtocolCoder LTV协议编码器
type LTVProtocolCoder struct {
	byteOrder  binary.ByteOrder // 网络字节序
	headerSize int              // 头部字节数
}

// NewLTVProtocolCoder 创建LTV协议编码器
func NewLTVProtocolCoder(isLittleEndian bool) *LTVProtocolCoder {
	instance := &LTVProtocolCoder{
		headerSize: LTVHeaderSize,
	}
	instance.byteOrder = binary.BigEndian
	if isLittleEndian {
		instance.byteOrder = binary.LittleEndian
	}
	return instance
}

// GetHeaderSize 获取头部字节数
func (ltv *LTVProtocolCoder) GetHeaderSize() int {
	return ltv.headerSize
}

// Decode 解码LTV数据包
func (ltv *LTVProtocolCoder) Decode(buffer []byte) (network.IPacket, uint32, error) {
	if len(buffer) < ltv.headerSize {
		// 数据不足以解析头部
		return nil, 0, nil
	}
	length := ltv.byteOrder.Uint32(buffer[0:4])  // 获取数据长度 L
	typeVal := ltv.byteOrder.Uint32(buffer[4:8]) // 获取数据包类型 T
	totalLen := length + uint32(ltv.headerSize)  // 计算总长度
	if len(buffer) < int(totalLen) {
		// 数据不够一整个包
		return nil, 0, nil
	}
	data := make([]byte, length)
	copy(data, buffer[ltv.headerSize:totalLen])
	ltvPkt := &LTVPacket{
		Header:   &LTVHeader{Length: length, Type: typeVal},
		Data:     data,
		TotalLen: totalLen,
	}
	return ltvPkt, totalLen, nil
}

// Encode 编码LTV数据包
func (ltv *LTVProtocolCoder) Encode(data network.IPacket) ([]byte, error) {
	ltvHeader, ok := data.GetHeader().(*LTVHeader)
	if !ok {
		return nil, ErrInvalidPacket
	}
	dataVal := data.GetData()
	length := len(dataVal)
	totalLen := length + ltv.headerSize
	buffer := bytes.NewBuffer(make([]byte, 0, totalLen))
	// L
	if err := binary.Write(buffer, ltv.byteOrder, ltvHeader.Length); err != nil {
		return nil, err
	}
	// T
	if err := binary.Write(buffer, ltv.byteOrder, ltvHeader.Type); err != nil {
		return nil, err
	}
	// V
	if _, err := buffer.Write(dataVal); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}
