package network

import (
	"errors"
	"net"
	"sync"
	"testing"
)

func TestSessionManagerRegistration(t *testing.T) {
	mgr := NewSessionManager[string](1)
	first := &testSession{id: 1}
	second := &testSession{id: 2}
	if err := mgr.Add(nil); err == nil {
		t.Fatal("应拒绝 nil 连接")
	}
	if err := mgr.Add(first); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Add(first); !errors.Is(err, ErrSessionIdExists) {
		t.Fatalf("重复登记: %v", err)
	}
	if err := mgr.Add(second); !errors.Is(err, ErrSessionLimitReached) {
		t.Fatalf("容量限制: %v", err)
	}
	if second.closes.Load() != 0 {
		t.Fatal("登记失败不应关闭调用方连接")
	}
	if conn, ok := mgr.Get(1); !ok || conn != first {
		t.Fatal("查询连接失败")
	}
	// 回调允许修改管理器，不能持有读锁。
	mgr.Range(func(conn Connection[string]) bool { mgr.Remove(conn.ID()); return false })
	if mgr.Len() != 0 || first.closes.Load() != 0 {
		t.Fatal("Remove 只应注销连接")
	}
	if err := mgr.Add(second); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Close(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Close(); err != nil {
		t.Fatal(err)
	}
	if second.closes.Load() != 1 {
		t.Fatal("重复关闭了连接")
	}
	if mgr.Len() != 1 {
		t.Fatal("Close 不应代替运行方注销连接")
	}
	if err := mgr.Add(first); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("关闭后登记: %v", err)
	}
	mgr.Remove(2)
	if mgr.Len() != 0 {
		t.Fatal("关闭后仍应允许注销")
	}
}

func TestSessionManagerCloseErrors(t *testing.T) {
	mgr := NewSessionManager[string](0)
	errA, errB := errors.New("close A"), errors.New("close B")
	for _, conn := range []*testSession{{id: 1, closeErr: errA}, {id: 2, closeErr: errB}} {
		if err := mgr.Add(conn); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		err := mgr.Close()
		if !errors.Is(err, errA) || !errors.Is(err, errB) {
			t.Fatalf("关闭错误丢失: %v", err)
		}
	}
}

func TestSessionManagerConcurrentAddClose(t *testing.T) {
	mgr := NewSessionManager[string](0)
	start := make(chan struct{})
	results := make(chan error, 64)
	sessions := make([]*testSession, 64)
	var wg sync.WaitGroup
	for i := range sessions {
		sessions[i] = &testSession{id: uint64(i + 1)}
		wg.Add(1)
		go func(conn *testSession) {
			defer wg.Done()
			<-start
			err := mgr.Add(conn)
			if err != nil {
				_ = conn.Close()
			}
			results <- err
		}(sessions[i])
	}
	wg.Add(1)
	go func() { defer wg.Done(); <-start; _ = mgr.Close() }()
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil && !errors.Is(err, net.ErrClosed) {
			t.Fatal(err)
		}
	}
	for _, conn := range sessions {
		if conn.closes.Load() != 1 {
			t.Fatalf("连接 %d 关闭次数: %d", conn.id, conn.closes.Load())
		}
	}
}
