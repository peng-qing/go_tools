package gpool

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"sync"
	"sync/atomic"

	"github.com/peng-qing/go_tools/common/container"
)

const (
	// TaskRunnerOpen 开启
	TaskRunnerOpen = iota
	// TaskRunnerClosed 关闭
	TaskRunnerClosed
)

var (
	ErrTaskRunnerBusy   = errors.New("task runner is busy")
	ErrTaskRunnerClosed = errors.New("task runner is closed")
)

type PanicHandler func(ctx context.Context, throwValue any)

// TaskFunc 任务函数
type TaskFunc func(ctx context.Context)

// Task 任务
type Task struct {
	Ctx      context.Context // 上下文
	TaskFunc TaskFunc        // 任务函数
}

// TaskRunner 任务执行器
type TaskRunner struct {
	panicHandler func(ctx context.Context, throwValue any) // 异常处理函数
	limitChan    chan container.None                       // 任务队列
	status       int32                                     // 状态
	wg           sync.WaitGroup                            // 等待组
}

// NewTaskRunner 创建任务执行器
// @param taskQueueSize 任务队列大小
// @param panicHandler 异常处理函数
// @return *TaskRunner
func NewTaskRunner(taskQueueSize int, panicHandler PanicHandler) *TaskRunner {
	if panicHandler == nil {
		panicHandler = func(ctx context.Context, throwValue any) {
			slog.Error("[TaskRunner] panic", "throwValue", throwValue)
		}
	}
	if taskQueueSize <= 0 {
		taskQueueSize = 1
	}

	return &TaskRunner{
		panicHandler: panicHandler,
		limitChan:    make(chan container.None, taskQueueSize),
		wg:           sync.WaitGroup{},
	}
}

// Submit 提交任务 队列满则阻塞
func (tr *TaskRunner) Submit(task Task) {
	if atomic.LoadInt32(&tr.status) == TaskRunnerClosed {
		return
	}
	tr.wg.Add(1)
	tr.limitChan <- container.None{}

	go func() {
		defer func() {
			<-tr.limitChan
			tr.wg.Done()
		}()
		defer func() {
			if err := recover(); err != nil {
				tr.panicHandler(task.Ctx, err)
			}
		}()
		task.TaskFunc(task.Ctx)
	}()
}

// SubmitImmediately 提交任务 队列满立即失败
func (tr *TaskRunner) SubmitImmediately(task Task) error {
	if atomic.LoadInt32(&tr.status) == TaskRunnerClosed {
		return ErrTaskRunnerClosed
	}
	tr.wg.Add(1)
	select {
	case tr.limitChan <- container.None{}:
	default:
		tr.wg.Done()
		return ErrTaskRunnerBusy
	}

	go func() {
		defer func() {
			tr.wg.Done()
			<-tr.limitChan
		}()
		defer func() {
			if err := recover(); err != nil {
				slog.Error("[TaskRunner] SubmitImmediately panic", "throwValue", err, "stack", string(debug.Stack()))
				tr.panicHandler(task.Ctx, err)
			}
		}()
		task.TaskFunc(task.Ctx)
	}()

	return nil
}

// Close 关闭任务执行器
func (tr *TaskRunner) Close() {
	if !atomic.CompareAndSwapInt32(&tr.status, TaskRunnerOpen, TaskRunnerClosed) {
		return
	}
	close(tr.limitChan)
	tr.wg.Wait()
}
