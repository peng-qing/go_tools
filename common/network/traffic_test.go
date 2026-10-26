package network

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

func TestTrafficStatisticsConcurrent(t *testing.T) {
	var statistics TrafficStatistics
	if got := statistics.Snapshot(); got != (TrafficSnapshot{}) {
		t.Fatalf("零值快照: %+v", got)
	}
	const workers, updates = 8, 200
	var wg sync.WaitGroup
	start, done := make(chan struct{}), make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < updates; j++ {
				statistics.IncrRead(7)
				statistics.IncrWrite(11)
			}
		}()
	}
	go func() { wg.Wait(); close(done) }()
	close(start)

	// 并发快照允许字段间存在时间差，但同一字段不应倒退。
	var previous TrafficSnapshot
	timer := time.NewTimer(testTimeout)
	defer timer.Stop()
	for {
		got := statistics.Snapshot()
		if got.ReadMessages < previous.ReadMessages || got.ReadBytes < previous.ReadBytes ||
			got.WriteMessages < previous.WriteMessages || got.WriteBytes < previous.WriteBytes {
			t.Fatalf("累计值发生倒退: previous=%+v got=%+v", previous, got)
		}
		previous = got
		select {
		case <-done:
			want := TrafficSnapshot{
				ReadMessages: workers * updates, ReadBytes: workers * updates * 7,
				WriteMessages: workers * updates, WriteBytes: workers * updates * 11,
			}
			if got := statistics.Snapshot(); got != want {
				t.Fatalf("并发累计: got %+v want %+v", got, want)
			}
			return
		case <-timer.C:
			t.Fatal("并发统计超时")
		default:
		}
	}
}

// 一次底层读取可以包含多个完整帧及一个截断帧，验证预读不被误计。
type testTrafficReader struct {
	Transport
	reader io.Reader
}

func (r testTrafficReader) Read(buffer []byte) (int, error) {
	return r.reader.Read(buffer)
}

func TestConnReadStatistics(t *testing.T) {
	tests := []struct {
		name           string
		data           []byte
		messages, size uint64
		wantErr        error
	}{
		{name: "empty_stream", wantErr: io.EOF},
		{name: "empty_message", data: []byte{0}, messages: 1, size: 1, wantErr: io.EOF},
		{name: "two_messages", data: []byte{1, 'a', 2, 'b', 'c'}, messages: 2, size: 5, wantErr: io.EOF},
		{name: "truncated_only", data: []byte{3, 'a'}, wantErr: io.ErrUnexpectedEOF},
		{name: "complete_then_truncated", data: []byte{1, 'a', 3, 'b'}, messages: 1, size: 2, wantErr: io.ErrUnexpectedEOF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport, _ := testPipe(t)
			conn := testNewConn(t, testTrafficReader{
				Transport: transport, reader: bytes.NewReader(tt.data),
			}, ConnDependencies[string]{Codec: testCodec{}}, ConnConfig{})
			done := testRunConn(t, conn)
			testReceive(t, done)
			if !errors.Is(conn.Err(), tt.wantErr) {
				t.Fatalf("终止原因: %v", conn.Err())
			}
			want := TrafficSnapshot{ReadMessages: tt.messages, ReadBytes: tt.size}
			if got := conn.Statistics(); got != want {
				t.Fatalf("读取统计: got %+v want %+v", got, want)
			}
		})
	}
}

type testTrafficWriter struct {
	Transport
	written chan struct{}
	partial bool
	err     error
}

func (w *testTrafficWriter) Write(data []byte) (int, error) {
	defer close(w.written)
	if w.partial {
		return len(data) - 1, w.err
	}
	return len(data), w.err
}
func (w *testTrafficWriter) Read([]byte) (int, error) {
	<-w.written
	return 0, io.EOF
}

func TestConnWriteStatistics(t *testing.T) {
	writeErr := errors.New("write failed")
	tests := []struct {
		name    string
		partial bool
		err     error
		want    TrafficSnapshot
	}{
		{name: "success", want: TrafficSnapshot{WriteMessages: 1, WriteBytes: 4}},
		{name: "short_write", partial: true},
		{name: "partial_with_error", partial: true, err: writeErr},
		{name: "full_count_with_error", err: writeErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport, _ := testPipe(t)
			writer := &testTrafficWriter{
				Transport: transport, written: make(chan struct{}), partial: tt.partial, err: tt.err,
			}
			conn := testNewConn(t, writer, ConnDependencies[string]{
				Codec: testCodec{},
				OnOpen: func(ctx context.Context, conn Connection[string]) error {
					return conn.Send(ctx, "abc")
				},
			}, ConnConfig{})
			done := testRunConn(t, conn)
			testReceive(t, done)
			if got := conn.Statistics(); got != tt.want {
				t.Fatalf("写入统计: got %+v want %+v", got, tt.want)
			}
		})
	}
}

func TestConnQueuedMessageNotCounted(t *testing.T) {
	transport, _ := testPipe(t)
	conn := testNewConn(t, transport, ConnDependencies[string]{Codec: testCodec{}}, ConnConfig{SendQueueSize: 1})
	if err := conn.TrySend("queued"); err != nil {
		t.Fatal(err)
	}
	if err := conn.TrySend("rejected"); !errors.Is(err, ErrSendQueueFull) {
		t.Fatal(err)
	}
	_ = conn.Close()
	if got := conn.Statistics(); got != (TrafficSnapshot{}) {
		t.Fatalf("未写出的消息不应计入流量: %+v", got)
	}
}

func TestConnHeartbeatStatistics(t *testing.T) {
	transport, peer := testPipe(t)
	business := make(chan struct{}, 1)
	conn := testNewConn(t, transport, ConnDependencies[string]{
		Codec: testCodec{}, Heartbeator: testHeartbeat{},
		Receiver: testReceiver(func(_ context.Context, _ Connection[string], packet string) {
			if packet != "data" {
				t.Errorf("心跳不应派发给业务: %q", packet)
			}
			business <- struct{}{}
		}),
	}, ConnConfig{})
	done := testRunConn(t, conn)
	_ = peer.SetDeadline(time.Now().Add(testTimeout))

	// 直接使用确定的帧，断言独立于测试编码器的长度计算。
	if _, err := peer.Write(append([]byte{7}, []byte("ping:42")...)); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 8)
	if _, err := io.ReadFull(peer, response); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(response, append([]byte{7}, []byte("pong:42")...)) {
		t.Fatalf("Pong 帧错误: %v", response)
	}
	// 收到的 Pong 也计入流量，但不产生回复。
	frames := append([]byte{7}, []byte("pong:42")...)
	frames = append(frames, 4, 'd', 'a', 't', 'a')
	if _, err := peer.Write(frames); err != nil {
		t.Fatal(err)
	}
	testReceive(t, business)
	_ = conn.Close()
	testReceive(t, done)
	want := TrafficSnapshot{ReadMessages: 3, ReadBytes: 21, WriteMessages: 1, WriteBytes: 8}
	if got := conn.Statistics(); got != want {
		t.Fatalf("心跳统计遗漏或重复: got %+v want %+v", got, want)
	}
}
