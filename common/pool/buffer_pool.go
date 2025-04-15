package pool

import (
	"io"
	"strconv"
	"time"
)

const (
	defaultSize = 1024
)

var (
	// 检查 Buffer 实现 io.Writer
	_ io.Writer = (*Buffer)(nil)
	// 检查 Buffer 实现 io.ByteWriter
	_ io.ByteWriter = (*Buffer)(nil)
	// 检查 Buffer 实现 io.StringWriter
	_ io.StringWriter = (*Buffer)(nil)
)

// Buffer 池化对象 不提供实例化方法
type Buffer struct {
	buf  []byte
	pool *BufferPool
}

// AppendByte 写入一个byte
func (b *Buffer) AppendByte(v byte) {
	b.buf = append(b.buf, v)
}

// AppendBytes 写入byte 数组
func (b *Buffer) AppendBytes(v []byte) {
	b.buf = append(b.buf, v...)
}

// AppendString 写入 string
func (b *Buffer) AppendString(v string) {
	b.buf = append(b.buf, v...)
}

// AppendInt 写入 int 10进制数
func (b *Buffer) AppendInt(v int64) {
	b.buf = strconv.AppendInt(b.buf, v, 10)
}

// AppendUint 写入 uint 10进制数
func (b *Buffer) AppendUint(v uint64) {
	b.buf = strconv.AppendUint(b.buf, v, 10)
}

// AppendBool 写入 bool
func (b *Buffer) AppendBool(v bool) {
	b.buf = strconv.AppendBool(b.buf, v)
}

// AppendFloat 写入 float64
func (b *Buffer) AppendFloat(v float64, bitSize int) {
	b.buf = strconv.AppendFloat(b.buf, v, 'f', -1, bitSize)
}

// AppendTime 写入 time.Time
func (b *Buffer) AppendTime(t time.Time, layout string) {
	b.buf = t.AppendFormat(b.buf, layout)
}

// Len 缓冲区数据长度
func (b *Buffer) Len() int {
	return len(b.buf)
}

// Bytes 获取缓冲区数据
func (b *Buffer) Bytes() []byte {
	return b.buf
}

// String 实现 fmt.Stringer 获取缓冲区数据 字符串形式
func (b *Buffer) String() string {
	return string(b.buf)
}

// Reset 重置缓冲区
func (b *Buffer) Reset() {
	b.buf = b.buf[:0]
}

// Write 实现 io.Writer
func (b *Buffer) Write(v []byte) (n int, err error) {
	b.AppendBytes(v)

	return len(v), nil
}

// WriteByte 实现 io.ByteWriter
func (b *Buffer) WriteByte(v byte) error {
	b.AppendByte(v)

	return nil
}

// WriteString 实现 io.StringWriter
func (b *Buffer) WriteString(v string) (n int, err error) {
	b.AppendString(v)

	return len(v), nil
}

// Free 返回对象池
func (b *Buffer) Free() {
	b.pool.put(b)
}

// TrimNewLine 去除末尾换行
func (b *Buffer) TrimNewLine() {
	if i := len(b.buf) - 1; i >= 0 && b.buf[i] == '\n' {
		b.buf = b.buf[:i]
	}
}

// BufferPool 缓冲区对象池
type BufferPool struct {
	pool *Pool[*Buffer]
}

// NewBufferPool 创建缓冲对象池
func NewBufferPool() *BufferPool {
	return &BufferPool{
		pool: NewPool[*Buffer](func() *Buffer {
			return &Buffer{
				buf:  make([]byte, 0, defaultSize),
				pool: nil,
			}
		}),
	}
}

// Get 获取缓冲区对象
func (bp *BufferPool) Get() *Buffer {
	buf := bp.pool.Get()
	// 重置
	buf.Reset()
	buf.pool = bp
	return buf
}

// 放回缓冲器 私有方法不对外
func (bp *BufferPool) put(buf *Buffer) {
	bp.pool.Put(buf)
}
