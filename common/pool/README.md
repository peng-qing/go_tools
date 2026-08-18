# pool

提供泛型对象池和可复用字节缓冲区。协程任务池见 [gpool/README.md](gpool/README.md)。

## 泛型对象池

`Pool[T]` 包装标准库 `sync.Pool`：

```go
type Item struct {
    Data []byte
}

items := pool.NewPool(func() *Item {
    return &Item{Data: make([]byte, 0, 1024)}
})

item := items.Get()
item.Data = append(item.Data, "hello"...)
item.Data = item.Data[:0]
items.Put(item)
```

对象归还前应由调用方重置状态。`sync.Pool` 中的对象可能随 GC 被释放，不能将其当作固定容量资源池。

## BufferPool

`Buffer` 实现 `io.Writer`、`io.ByteWriter`、`io.StringWriter` 和 `fmt.Stringer`，并支持追加基础类型：

```go
bp := pool.NewBufferPool()
buf := bp.Get()
defer buf.Free()

buf.AppendString("request-")
buf.AppendInt(1001)
buf.AppendByte('\n')
buf.TrimNewLine()

fmt.Println(buf.String())
```

主要方法：

- `AppendByte/Bytes/String`
- `AppendInt/Uint/Bool/Float/Time`
- `Write/WriteByte/WriteString`
- `Len/Bytes/String`
- `Reset/TrimNewLine/Free`

`Get` 会自动重置缓冲区；使用结束后调用 `Free` 归还池。归还后不要继续访问 `Buffer` 或其 `Bytes` 返回的底层切片。
