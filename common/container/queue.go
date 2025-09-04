package container

const (
	// 初始化容量
	initCapacity = 8
)

// Queue 队列 线程不安全
type Queue[T any] struct {
	// 队列数据 采用slice
	// 对比 list.List 结构更简单，维护成本低
	// 缺点是如果元素过大 slice 频繁拷贝会耗费更多性能
	data     []T
	begin    int
	end      int
	capacity int
}

// NewQueue 创建队列
func NewQueue[T any]() *Queue[T] {
	return &Queue[T]{
		data:     make([]T, initCapacity),
		capacity: initCapacity,
		begin:    0,
		end:      0,
	}
}

// Empty 判断队列是否为空
func (q *Queue[T]) Empty() bool {
	return q.Size() <= 0
}

// Size 获取队列长度
func (q *Queue[T]) Size() int {
	return q.end - q.begin
}

// Clear 清空队列
func (q *Queue[T]) Clear() {
	q.begin = 0
	q.end = 0
	q.data = make([]T, initCapacity)
	q.capacity = initCapacity
}

// Value 获取队列数据
func (q *Queue[T]) Value() []T {
	values := make([]T, 0, q.Size())
	for i := q.begin; i < q.end; i++ {
		values = append(values, q.data[i])
	}
	return values
}

// Push 入队
func (q *Queue[T]) Push(val T) {
	if q.end >= q.capacity {
		// 扩容
		q.expend()
	}
	q.data[q.end] = val
	q.end++
}

// Pop 出队
func (q *Queue[T]) Pop() (val T) {
	if q.Empty() {
		return
	}
	val = q.data[q.begin]
	q.begin++

	// 缩容
	if q.begin >= q.capacity/2 {
		q.shrink()
	}
	return
}

// 扩容
func (q *Queue[T]) expend() {
	// 优先移动元素到首部
	if q.begin > 0 {
		length := q.Size()
		copy(q.data, q.data[q.begin:q.end])
		q.begin = 0
		q.end = length
		return
	}
	newCapacity := q.capacity * 2
	if q.capacity >= 1024 {
		newCapacity = q.capacity/4 + q.capacity
	}
	newData := make([]T, newCapacity)
	length := copy(newData, q.data[q.begin:q.end])
	q.begin = 0
	q.end = length
	q.data = newData
	q.capacity = newCapacity
}

// 缩容
func (q *Queue[T]) shrink() {
	length := q.Size()
	if length <= q.capacity/4 && q.capacity > initCapacity {
		newCapacity := q.capacity / 2
		if newCapacity < initCapacity {
			newCapacity = initCapacity
		}
		newData := make([]T, newCapacity)
		copy(newData, q.data[q.begin:q.end])
		q.begin = 0
		q.end = length
		q.data = newData
		q.capacity = newCapacity
	}
}
