package network

import "sync/atomic"

// TrafficSnapshot 当前连接的累计统计快照
type TrafficSnapshot struct {
	ReadMessages  uint64 // 成功解码消息数 包括心跳包
	ReadBytes     uint64 // 成功读取字节数
	WriteMessages uint64 // 成功写入消息数 包括心跳包
	WriteBytes    uint64 // 成功写入字节数
}

// TrafficStatistics 当前连接的累计统计
type TrafficStatistics struct {
	readMessages  atomic.Uint64 // 成功解码消息数 包括心跳包
	readBytes     atomic.Uint64 // 成功读取字节数
	writeMessages atomic.Uint64 // 成功写入消息数 包括心跳包
	writeBytes    atomic.Uint64 // 成功写入字节数
}

// Snapshot 获取统计快照
func (ts *TrafficStatistics) Snapshot() TrafficSnapshot {
	return TrafficSnapshot{
		ReadMessages:  ts.readMessages.Load(),
		ReadBytes:     ts.readBytes.Load(),
		WriteMessages: ts.writeMessages.Load(),
		WriteBytes:    ts.writeBytes.Load(),
	}
}

// SetRead 累计一条成功解码消息及其编码字节数
func (ts *TrafficStatistics) IncrRead(rl uint64) {
	ts.readMessages.Add(1)
	ts.readBytes.Add(rl)
}

// SetWrite 累计一条完整写入消息及其编码字节数
func (ts *TrafficStatistics) IncrWrite(wl uint64) {
	ts.writeMessages.Add(1)
	ts.writeBytes.Add(wl)
}
