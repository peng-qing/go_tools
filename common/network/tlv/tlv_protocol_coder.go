package tlv

import (
	"bytes"
	"encoding/binary"

	"go_tools/common/network"
)

const (
	// TLVHeaderSize tlv格式包头大小
	// |  length(4byte)  |  type(2byte)  |
	TLVHeaderSize = 6
)

var (
	// TLVProtocolCoder 实现了IProtocolCoder接口
	_ network.IProtocolCoder = (*TLVProtocolCoder)(nil)
)

// TLVProtocolCoder TLV协议编码器
type TLVProtocolCoder struct {
	byteOrder  binary.ByteOrder // 网络字节序
	headerSize int              // 头部字节数
}

// NewTLVProtocolCoder 创建TLV协议编码器
func NewTLVProtocolCoder(isLittleEndian bool) *TLVProtocolCoder {
	instance := &TLVProtocolCoder{
		headerSize: TLVHeaderSize,
	}
	instance.byteOrder = binary.BigEndian
	if isLittleEndian {
		instance.byteOrder = binary.LittleEndian
	}
	return instance
}

func (tlv *TLVProtocolCoder) GetHeaderSize() int {
	return tlv.headerSize
}

// Decode 解码TLV数据包
func (tlv *TLVProtocolCoder) Decode(buffer []byte) (network.IPacket, uint32, error) {
	if len(buffer) < tlv.headerSize {
		// 数据不足以解析头部
		return nil, 0, nil
	}
	typeVal := tlv.byteOrder.Uint16(buffer[0:2]) // 获取数据包类型 T
	length := tlv.byteOrder.Uint32(buffer[2:6])  // 获取数据长度 L
	totalLen := length + uint32(tlv.headerSize)  // 计算总长度
	if len(buffer) < int(totalLen) {
		// 数据不够一整个包
		return nil, 0, nil
	}
	data := make([]byte, length)
	copy(data, buffer[tlv.headerSize:totalLen])
	tlvPkt := &TLVPacket{
		Header:   &TLVHeader{Length: length, Type: typeVal},
		Data:     data,
		TotalLen: totalLen,
	}
	return tlvPkt, totalLen, nil
}

// Encode 编码TLV数据包
func (tlv *TLVProtocolCoder) Encode(data network.IPacket) ([]byte, error) {
	tlvHeader, ok := data.GetHeader().(*TLVHeader)
	if !ok {
		return nil, network.ErrInvalidPacket
	}
	dataVal := data.GetData()
	length := len(dataVal)
	totalLen := length + tlv.headerSize
	buffer := bytes.NewBuffer(make([]byte, 0, totalLen))

	// T
	if err := binary.Write(buffer, tlv.byteOrder, tlvHeader.Type); err != nil {
		return nil, err
	}
	// L
	if err := binary.Write(buffer, tlv.byteOrder, tlvHeader.Length); err != nil {
		return nil, err
	}
	// V
	if _, err := buffer.Write(dataVal); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}
