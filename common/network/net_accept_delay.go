package network

import "time"

const (
	// 最大延迟时间
	maxDelay = 1 * time.Second
)

var AcceptDelay *delay

func init() {
	AcceptDelay = &delay{duration: 0}
}

type delay struct {
	duration time.Duration
}

// Delay 延迟
func (d *delay) Delay() {
	d.increment()
	if d.duration > 0 {
		time.Sleep(d.duration)
	}
}

// increment 增加延迟
func (d *delay) increment() {
	if d.duration <= 0 {
		d.duration = 5 * time.Millisecond
		return
	}
	d.duration = d.duration * 2
	if d.duration > maxDelay {
		d.duration = maxDelay
	}
}

// Reset 重置延迟
func (d *delay) Reset() {
	d.duration = 0
}
