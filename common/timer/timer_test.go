package timer

import (
	"container/heap"
	"reflect"
	"testing"
)

type recordingTimeout struct {
	calls []int64
}

func (r *recordingTimeout) TimeOut(now int64) {
	r.calls = append(r.calls, now)
}

func TestTimerQueueHeapOperations(t *testing.T) {
	queue := make(TimerQueue, 0, 4)
	queue.Push("not a timer")
	if queue.Len() != 0 || queue.Pop() != nil {
		t.Fatal("invalid push or empty pop changed queue")
	}

	a := &Timer{id: 1, end: 30}
	b := &Timer{id: 2, end: 10}
	c := &Timer{id: 3, end: 20}
	heap.Push(&queue, a)
	heap.Push(&queue, b)
	heap.Push(&queue, c)
	if queue.Len() != 3 || !queue.Less(0, 1) {
		t.Fatalf("unexpected heap: %#v", queue)
	}

	queue.Swap(1, 2)
	if queue[1].index != 1 || queue[2].index != 2 {
		t.Fatal("Swap() did not update indexes")
	}
	heap.Init(&queue)

	var ids []int64
	for queue.Len() > 0 {
		ids = append(ids, heap.Pop(&queue).(*Timer).id)
	}
	expectedIDs := []int64{2, 3, 1}
	t.Logf("actual: %v", ids)
	t.Logf("expected: %v", expectedIDs)
	if !reflect.DeepEqual(ids, expectedIDs) {
		t.Fatalf("pop order = %v", ids)
	}
	if a.index != -1 || b.index != -1 || c.index != -1 {
		t.Fatal("Pop() did not invalidate timer indexes")
	}
}

func TestTimerManagerAddRemoveRunAndRepeat(t *testing.T) {
	tm := NewTimerManager(3)
	first := &recordingTimeout{}
	repeating := &recordingTimeout{}
	removed := &recordingTimeout{}

	firstID := tm.AddTimer(first, 10, 0)
	repeatingID := tm.AddTimer(repeating, 5, 10)
	removedID := tm.AddTimer(removed, 1, 0)
	if firstID == 0 || repeatingID == 0 || removedID == 0 {
		t.Fatal("AddTimer() returned zero")
	}
	if got := tm.AddTimer(first, 0, 0); got != 0 {
		t.Fatalf("AddTimer() over capacity = %d", got)
	}
	tm.RemoveTimer(removedID)
	tm.RemoveTimer(9999)

	checks, calls := tm.Run(10, 1)
	if checks != 1 || calls != 1 || !reflect.DeepEqual(repeating.calls, []int64{10}) {
		t.Fatalf("limited Run() = (%d, %d), repeating calls %v", checks, calls, repeating.calls)
	}
	if len(first.calls) != 0 || len(removed.calls) != 0 {
		t.Fatalf("unexpected calls: first %v removed %v", first.calls, removed.calls)
	}

	checks, calls = tm.Run(10, 0)
	if checks != 2 || calls != 1 || !reflect.DeepEqual(first.calls, []int64{10}) {
		t.Fatalf("second Run() = (%d, %d), first calls %v", checks, calls, first.calls)
	}
	checks, calls = tm.Run(20, 0)
	expectedCallbacks := []int64{10, 20}
	t.Logf("actual: checks=%d, calls=%d, callbacks=%v", checks, calls, repeating.calls)
	t.Logf("expected: checks=2, calls=1, callbacks=%v", expectedCallbacks)
	if checks != 2 || calls != 1 || !reflect.DeepEqual(repeating.calls, expectedCallbacks) {
		t.Fatalf("repeat Run() = (%d, %d), calls %v", checks, calls, repeating.calls)
	}
}

func TestNewTimerManagerUsesDefaultSize(t *testing.T) {
	tm := NewTimerManager(0)
	t.Logf("actual: %d", cap(tm.tq))
	t.Logf("expected: %d", DefaultTimerQueueSize)
	if cap(tm.tq) != DefaultTimerQueueSize {
		t.Fatalf("timer queue capacity = %d", cap(tm.tq))
	}
}
