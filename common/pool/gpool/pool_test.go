package gpool

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestPoolExecutesQueuedJobsBeforeClose(t *testing.T) {
	p := NewPool(2, 8)
	var executed atomic.Int32

	for i := 0; i < 20; i++ {
		p.Submit(Job{
			WorkerID: i % 2,
			Ctx:      context.Background(),
			Handler: func(context.Context) error {
				executed.Add(1)
				return nil
			},
		})
	}

	p.Close()

	if got := executed.Load(); got != 20 {
		t.Fatalf("executed %d jobs, want 20", got)
	}
}

func TestPoolRecoversJobPanicAndContinues(t *testing.T) {
	p := NewPool(1, 2)
	completed := make(chan struct{})

	p.Submit(Job{
		WorkerID: 0,
		Handler: func(context.Context) error {
			panic("test panic")
		},
	})
	p.Submit(Job{
		WorkerID: 0,
		Handler: func(context.Context) error {
			close(completed)
			return nil
		},
	})

	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("worker did not execute the job after a panic")
	}

	p.Close()
}

func TestPoolNormalizesInvalidSizes(t *testing.T) {
	p := NewPool(0, -1)
	completed := make(chan struct{})

	p.Submit(Job{
		WorkerID: -1,
		Handler: func(context.Context) error {
			close(completed)
			return nil
		},
	})
	p.Close()

	select {
	case <-completed:
	default:
		t.Fatal("job was not executed")
	}
}

func TestPoolCloseIsIdempotent(t *testing.T) {
	p := NewPool(1, 1)
	p.Close()
	p.Close()
}

func TestTaskRunnerLimitsConcurrency(t *testing.T) {
	const limit = int32(2)
	runner := NewTaskRunner(int(limit), nil)

	var active atomic.Int32
	var maximum atomic.Int32

	for i := 0; i < 10; i++ {
		runner.Submit(Task{TaskFunc: func(context.Context) {
			current := active.Add(1)
			for {
				old := maximum.Load()
				if current <= old || maximum.CompareAndSwap(old, current) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			active.Add(-1)
		}})
	}

	runner.Close()

	if got := maximum.Load(); got > limit {
		t.Fatalf("maximum concurrency = %d, want <= %d", got, limit)
	}
	if got := maximum.Load(); got == 0 {
		t.Fatal("no task was executed")
	}
}

func TestTaskRunnerSubmitImmediatelyReturnsBusy(t *testing.T) {
	runner := NewTaskRunner(1, nil)
	started := make(chan struct{})
	release := make(chan struct{})

	err := runner.SubmitImmediately(Task{TaskFunc: func(context.Context) {
		close(started)
		<-release
	}})
	if err != nil {
		t.Fatalf("first SubmitImmediately() error = %v", err)
	}
	<-started

	err = runner.SubmitImmediately(Task{TaskFunc: func(context.Context) {}})
	if !errors.Is(err, ErrTaskRunnerBusy) {
		t.Fatalf("second SubmitImmediately() error = %v, want %v", err, ErrTaskRunnerBusy)
	}

	close(release)
	runner.Close()
}

func TestTaskRunnerReleasesTokenAfterPanic(t *testing.T) {
	panicValue := make(chan any, 1)
	runner := NewTaskRunner(1, func(_ context.Context, value any) {
		panicValue <- value
	})

	runner.Submit(Task{TaskFunc: func(context.Context) {
		panic("test panic")
	}})

	select {
	case value := <-panicValue:
		if value != "test panic" {
			t.Fatalf("panic value = %v, want test panic", value)
		}
	case <-time.After(time.Second):
		t.Fatal("panic handler was not called")
	}

	completed := make(chan struct{})
	go func() {
		runner.Submit(Task{TaskFunc: func(context.Context) {
			close(completed)
		}})
	}()

	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("token was not released after task panic")
	}

	runner.Close()
}

func TestTaskRunnerCloseIsIdempotentAndRejectsImmediateSubmit(t *testing.T) {
	runner := NewTaskRunner(1, nil)
	runner.Close()
	runner.Close()

	err := runner.SubmitImmediately(Task{TaskFunc: func(context.Context) {}})
	if !errors.Is(err, ErrTaskRunnerClosed) {
		t.Fatalf("SubmitImmediately() error = %v, want %v", err, ErrTaskRunnerClosed)
	}
}
