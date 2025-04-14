package container

type None struct{}

// Container 容器接口
type Container[T any] interface {
	Empty() bool
	Size() int
	Clear()
	Value() []T
}
