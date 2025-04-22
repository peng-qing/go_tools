package options

// Option Options 接口
type Option[T any] interface {
	Apply(t *T)
}

// WrapperOptions 包装Options
type WrapperOptions[T any] func(t *T)

// Apply 实现Options接口
func (opt WrapperOptions[T]) Apply(t *T) {
	opt(t)
}
