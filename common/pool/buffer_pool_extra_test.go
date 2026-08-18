package pool

import (
	"reflect"
	"testing"
	"time"
)

func TestBufferAppendScalarValuesAndBytes(t *testing.T) {
	bp := NewBufferPool()
	buf := bp.Get()
	defer buf.Free()

	when := time.Date(2026, time.August, 18, 9, 8, 7, 0, time.UTC)
	buf.AppendBytes([]byte("n="))
	buf.AppendInt(-12)
	buf.AppendString(",u=")
	buf.AppendUint(34)
	buf.AppendString(",b=")
	buf.AppendBool(true)
	buf.AppendString(",f=")
	buf.AppendFloat(1.25, 64)
	buf.AppendString(",t=")
	buf.AppendTime(when, time.RFC3339)

	want := "n=-12,u=34,b=true,f=1.25,t=2026-08-18T09:08:07Z"
	got := buf.String()
	t.Logf("actual: %q", got)
	t.Logf("expected: %q", want)
	if got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(buf.Bytes(), []byte(want)) {
		t.Fatalf("Bytes() = %q", buf.Bytes())
	}
}

func TestBufferTrimNewLineNoOp(t *testing.T) {
	bp := NewBufferPool()
	buf := bp.Get()
	defer buf.Free()

	buf.TrimNewLine()
	buf.AppendString("unchanged")
	buf.TrimNewLine()
	got := buf.String()
	t.Logf("actual: %q", got)
	t.Logf("expected: %q", "unchanged")
	if got != "unchanged" {
		t.Fatalf("TrimNewLine() changed %q", got)
	}
}
