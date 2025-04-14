package timer

import "container/heap"

var (
	// 断言 检查实现 heap.Interface
	_ heap.Interface = (*TimerQueue)(nil)
)

// TimeOuter 定时器回调接口
type TimeOuter interface {
	TimeOut(nowTm int64)
}

// Timer 定时器结构体
type Timer struct {
	TimeOuter
	id       int64 // 定时器ID
	end      int64 // 结束时间
	interval int64 // 间隔时间
	index    int   // 索引
}

// TimerQueue 定时器队列结构体
type TimerQueue []*Timer

// Len 长度
func (tq TimerQueue) Len() int {
	return len(tq)
}

// Less 比较
func (tq TimerQueue) Less(i, j int) bool {
	return tq[i].end < tq[j].end
}

// Swap 交换
func (tq TimerQueue) Swap(i, j int) {
	tq[i], tq[j] = tq[j], tq[i]
	tq[i].index = i
	tq[j].index = j
}

// Push 插入
func (tq *TimerQueue) Push(x any) {
	timer, ok := x.(*Timer)
	if !ok {
		return
	}
	// append 会扩容需要大量拷贝数据
	// 保证底层数组预分配合适的 cap 的情况下在底层数组上新建 slice
	// *tq = append(*tq, timer)

	tmp := *tq
	length := len(tmp)
	tmp = tmp[:length+1]
	timer.index = length
	tmp[length] = timer
	*tq = tmp
}

// Pop 弹出
func (tq *TimerQueue) Pop() any {
	tmp := *tq
	length := len(tmp)
	if length == 0 {
		return nil
	}
	timer := tmp[length-1]
	tmp[length-1] = nil
	// 标记失效
	timer.index = -1
	*tq = tmp[:length-1]

	return timer
}
