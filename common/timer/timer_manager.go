package timer

import (
	"container/heap"
	"log/slog"
	"runtime/debug"
	"sync/atomic"

	"github.com/peng-qing/go_tools/common/container"
)

const (
	DefaultTimerQueueSize = 1024
)

type TimerManager struct {
	id    int64                    // 定时器唯一ID
	tq    TimerQueue               // 定时器队列
	queue *container.Queue[*Timer] // 定时器执行队列
}

// NewTimerManager 创建定时器管理器
func NewTimerManager(size int) *TimerManager {
	if size <= 0 {
		size = DefaultTimerQueueSize
	}
	return &TimerManager{
		id:    0,
		tq:    make(TimerQueue, 0, size),
		queue: container.NewQueue[*Timer](),
	}
}

// AddTimer 添加定时器
// @param timeOuter 定时器回调
// @param end 定时器结束时间
// @param interval 定时器间隔时间
// @return 定时器ID
func (tm *TimerManager) AddTimer(timeOuter TimeOuter, end int64, interval int64) int64 {
	if cap(tm.tq) <= len(tm.tq) {
		slog.Warn("[TimerManager] AddTimer timer is full", "timerOuter", timeOuter, "end", end, "interval", interval)
		return 0
	}
	id := atomic.AddInt64(&tm.id, 1)
	timer := &Timer{
		TimeOuter: timeOuter,
		end:       end,
		id:        id,
		interval:  interval,
	}

	heap.Push(&tm.tq, timer)
	return timer.id
}

// RemoveTimer 移除定时器
func (tm *TimerManager) RemoveTimer(timerId int64) {
	for _, timer := range tm.tq {
		if timer.id == timerId {
			heap.Remove(&tm.tq, timer.index)
			break
		}
	}
}

// Run 执行定时器
// @param nowTm 当前时间
// @param limit 最大执行次数
// @return 检查数量, 执行数量
func (tm *TimerManager) Run(nowTm int64, limit int) (uint32, uint32) {
	defer func() {
		if err := recover(); err != nil {
			slog.Error("[TimerManager] Run panic", "err", err, "stack", debug.Stack())
		}
	}()

	checkCount := uint32(0)
	callCount := uint32(0)

	for tm.tq.Len() > 0 {
		checkCount++
		// 小根堆 根节点最小
		tmp := tm.tq[0]
		if tmp.end > nowTm {
			// 未到触发时间
			break
		}
		timer := heap.Pop(&tm.tq).(*Timer)
		tm.queue.Push(timer)
		// 存在 间隔执行时间
		if timer.interval > 0 {
			// 重新加入定时器
			// 避免不停止服务修改服务器时间导致频繁触发
			// timer.end += timer.interval
			timer.end = nowTm + timer.interval
			heap.Push(&tm.tq, timer)
		}
		if limit > 0 && tm.queue.Size() >= limit {
			// 执行数量达到上限
			break
		}
	}

	for tm.queue.Size() > 0 {
		// 执行定时器回调
		callCount++
		timer := tm.queue.Pop()
		timer.TimeOut(nowTm)
	}

	return checkCount, callCount
}
