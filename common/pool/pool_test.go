package pool

import (
	"sync/atomic"
	"testing"
)

func TestPoolGet(t *testing.T) {
	var created atomic.Int32
	p := NewPool(func() *int {
		created.Add(1)
		value := 42
		return &value
	})

	value := p.Get()
	if value == nil || *value != 42 {
		t.Fatalf("Get() = %v, want pointer to 42", value)
	}
	if created.Load() != 1 {
		t.Fatalf("factory called %d times, want 1", created.Load())
	}

	p.Put(value)
}

func TestNewPoolWithNilFactoryPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewPool(nil) did not panic")
		}
	}()

	_ = NewPool[int](nil)
}

func TestBufferWriteAndReset(t *testing.T) {
	bp := NewBufferPool()
	buf := bp.Get()

	buf.AppendString("value=")
	buf.AppendInt(42)
	buf.AppendByte('\n')
	buf.TrimNewLine()

	if got, want := buf.String(), "value=42"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if got, want := buf.Len(), len("value=42"); got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}

	buf.Free()

	reused := bp.Get()
	if reused.Len() != 0 {
		t.Fatalf("buffer was not reset: Len() = %d", reused.Len())
	}
	reused.Free()
}

func TestBufferWriterInterfaces(t *testing.T) {
	bp := NewBufferPool()
	buf := bp.Get()
	defer buf.Free()

	if n, err := buf.Write([]byte("go")); err != nil || n != 2 {
		t.Fatalf("Write() = (%d, %v), want (2, nil)", n, err)
	}
	if err := buf.WriteByte('-'); err != nil {
		t.Fatalf("WriteByte() error = %v", err)
	}
	if n, err := buf.WriteString("tools"); err != nil || n != 5 {
		t.Fatalf("WriteString() = (%d, %v), want (5, nil)", n, err)
	}

	if got, want := buf.String(), "go-tools"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}
