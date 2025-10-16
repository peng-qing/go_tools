package gpool

import (
	"log/slog"

	"github.com/peng-qing/go_tools/common/container"
)

// worker 工人
type worker struct {
	poolID   uint64
	workerID int
	jobChan  chan Job
	stopChan chan container.None
}

// newWorker 创建工人
func newWorker(poolID uint64, workerID int, jobQueeueSize int) *worker {
	return &worker{
		poolID:   poolID,
		workerID: workerID,
		jobChan:  make(chan Job, jobQueeueSize),
		stopChan: make(chan container.None),
	}
}

// process 处理任务
func (w *worker) process() {
	slog.Info("[worker] process starting", slog.Uint64("poolID", w.poolID), slog.Int("workerID", w.workerID))
	for {
		select {
		case job := <-w.jobChan:
			// 正常处理工作队列
			if err := job.Handler(job.Ctx); err != nil {
				slog.Error("[worker] process job handler error", slog.Uint64("poolID", w.poolID), slog.Int("workerID", w.workerID), slog.Any("error", err))
			}
		case <-w.stopChan:
			slog.Info("[worker] process stopping", slog.Uint64("poolID", w.poolID), slog.Int("workerID", w.workerID))
			// 停止接收新任务
			close(w.jobChan)
			close(w.stopChan)
			// 执行剩余任务
			for job := range w.jobChan {
				if err := job.Handler(job.Ctx); err != nil {
					slog.Error("[worker] process job handler error", slog.Uint64("poolID", w.poolID), slog.Int("workerID", w.workerID), slog.Any("error", err))
				}
			}
			slog.Info("[worker] process stopped", slog.Uint64("poolID", w.poolID), slog.Int("workerID", w.workerID))
			return
		}
	}
}
