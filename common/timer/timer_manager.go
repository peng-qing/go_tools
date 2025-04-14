package timer

type TimerManager struct {
	id int64      // 定时器唯一ID
	tq TimerQueue // 定时器队列
}
