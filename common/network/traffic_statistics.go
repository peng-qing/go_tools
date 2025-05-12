package network

import "sync/atomic"

// TrafficStatistics 流量分析
type TrafficStatistics struct {
	readMsgCount  uint32
	writeMsgCount uint32
	readMsgSize   uint64
	writeMsgSize  uint64
}

// NewTrafficStatistics 创建流量分析对象
func NewTrafficStatistics() *TrafficStatistics {
	return &TrafficStatistics{}
}

// IncrRead 增加读取消息
func (ts *TrafficStatistics) IncrRead(msgSize uint32) {
	atomic.AddUint32(&ts.readMsgCount, 1)
	atomic.AddUint64(&ts.readMsgSize, uint64(msgSize))
}

// IncrWrite 增加写入消息
func (ts *TrafficStatistics) IncrWrite(msgSize uint32) {
	atomic.AddUint32(&ts.writeMsgCount, 1)
	atomic.AddUint64(&ts.writeMsgSize, uint64(msgSize))
}

// Reset 重置
func (ts *TrafficStatistics) Reset() {
	atomic.StoreUint32(&ts.readMsgCount, 0)
	atomic.StoreUint64(&ts.readMsgSize, 0)
	atomic.StoreUint32(&ts.writeMsgCount, 0)
	atomic.StoreUint64(&ts.writeMsgSize, 0)
}

// Get 获取
func (ts *TrafficStatistics) Get() (readMsgCount uint32, writeMsgCount uint32, readMsgSize uint64, writeMsgSize uint64) {
	readMsgSize = atomic.LoadUint64(&ts.readMsgSize)
	readMsgCount = atomic.LoadUint32(&ts.readMsgCount)
	writeMsgSize = atomic.LoadUint64(&ts.writeMsgSize)
	writeMsgCount = atomic.LoadUint32(&ts.writeMsgCount)
	return
}

// TS 全局流量分析对象
var TS = NewTrafficStatistics()
