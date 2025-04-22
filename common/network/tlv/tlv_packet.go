package tlv

import "go_tools/common/network"

var (
	// 检查实现 IPacket
	_ network.IPacket = (*TLVPacket)(nil)
)

type TLVHeader struct {
	Type   uint16 // TLV格式包类型
	Length uint32 // TLV格式包长度
}

// TLVPacket TLV格式包
type TLVPacket struct {
	Header   *TLVHeader // TLV格式包头
	Data     []byte     // TLV格式包数据域
	TotalLen uint32     // TLV格式包总长度
}

// GetData 获取TLV格式包数据域
func (p *TLVPacket) GetData() []byte {
	return p.Data
}

// GetLength 获取TLV格式包长度
func (p *TLVPacket) GetLength() uint32 {
	return p.Header.Length
}

// GetHeader 获取TLV格式包头
func (p *TLVPacket) GetHeader() any {
	return p.Header
}

// GetTotalLength 获取TLV格式包总长度
func (p *TLVPacket) GetTotalLength() uint32 {
	return p.TotalLen
}
