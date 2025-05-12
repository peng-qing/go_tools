package ltv

import "go_tools/common/network"

var (
	// 检查实现 IPacket
	_ network.IPacket = (*LTVPacket)(nil)
)

type LTVHeader struct {
	Type   uint32 // LTV格式包类型
	Length uint32 // LTV格式包长度
}

// LTVPacket LTV格式包
type LTVPacket struct {
	Header   *LTVHeader // LTV格式包头
	Data     []byte     // LTV格式包数据域
	TotalLen uint32     // LTV格式包总长度
}

// NewLTVPacket 创建LTV格式包
func NewLTVPacket(msgType uint32, data []byte) network.IPacket {
	return &LTVPacket{
		Header: &LTVHeader{
			Type:   msgType,
			Length: uint32(len(data)),
		},
		Data:     data,
		TotalLen: uint32(len(data) + LTVHeaderSize),
	}
}

// GetData 获取LTV格式包数据域
func (l *LTVPacket) GetData() []byte {
	return l.Data
}

// GetLength 获取LTV格式包长度
func (l *LTVPacket) GetLength() uint32 {
	return l.Header.Length
}

// GetHeader 获取LTV格式包头
func (l *LTVPacket) GetHeader() any {
	return l.Header
}

// GetTotalLength 获取LTV格式包总长度
func (l *LTVPacket) GetTotalLength() uint32 {
	return l.TotalLen
}
