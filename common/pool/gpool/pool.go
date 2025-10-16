package gpool

import (
	"context"
	"log/slog"
	"math/rand"
	"runtime/debug"
	"sync"
	"sync/atomic"
)

var (
	// 协程池ID生成器
	poolIDGenerator uint64
)

const (
	PoolStatusOpen  = 0 // 协程池状态 打开
	PoolStatusClose = 1 // 协程池状态 关闭
)

// Job 任务
type Job struct {
	WorkerID int
	Ctx      context.Context
	Handler  func(context.Context) error
}

// 协程池
type Pool struct {
	id       uint64         // 协程池ID
	status   int32          // 协程池状态
	capacity int            // 最大协程数
	workers  []*worker      // 工人列表
	wg       sync.WaitGroup // 等待组
}

// NewPool 创建协程池
// @param capacity 最大协程数
// @param jobQueueSize 任务队列大小
// @return *Pool
func NewPool(capacity int, jobQueueSize int) *Pool {
	pool := &Pool{
		id:       atomic.AddUint64(&poolIDGenerator, 1),
		status:   PoolStatusOpen,
		capacity: capacity,
		workers:  make([]*worker, 0, capacity),
		wg:       sync.WaitGroup{},
	}
	for i := range capacity {
		pool.workers[i] = newWorker(pool.id, i, jobQueueSize)
		pool.wg.Add(1)
		go func() {
			defer pool.wg.Done()
			defer func() {
				if err := recover(); err != nil {
					slog.Error("[Pool] process worker panic", slog.Uint64("poolID", pool.id), slog.Int("workerID", i), slog.Any("error", err), slog.String("stack", string(debug.Stack())))
				}
			}()
			pool.workers[i].process()
		}()
	}

	return pool
}

// Close 关闭协程池
func (p *Pool) Close() {
	slog.Info("[Pool] Close starting", slog.Uint64("poolID", p.id))
	if atomic.LoadInt32(&p.status) == PoolStatusClose {
		// 已经关闭了
		return
	}
	atomic.StoreInt32(&p.status, PoolStatusClose)
	for _, worker := range p.workers {
		close(worker.stopChan)
	}
	p.wg.Wait()
	slog.Info("[Pool] Close success", slog.Uint64("poolID", p.id))
}

// Submit 提交任务
func (p *Pool) Submit(job Job) {
	if atomic.LoadInt32(&p.status) == PoolStatusClose {
		slog.Error("[Pool] Submit pool already closed", slog.Uint64("poolID", p.id))
		return
	}
	// workerID < 0 无序负载均衡
	if job.WorkerID < 0 {
		index := rand.Intn(p.capacity)
		p.workers[index].jobChan <- job
	} else {
		// workerID >= 0 有序负载均衡
		index := job.WorkerID % p.capacity
		p.workers[index].jobChan <- job
	}
}
