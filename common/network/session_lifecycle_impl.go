package network

import "context"

var (
	_ SessionLifecycle[any] = (*NopSessionLifecycle[any])(nil)
)

// NopSessionLifecycle 是 SessionLifecycle 的无副作用空实现 避免频繁判断nil
type NopSessionLifecycle[P any] struct{}

func (NopSessionLifecycle[P]) OnReady(context.Context, Connection[P]) {}
func (NopSessionLifecycle[P]) OnClosed(Connection[P], error)          {}
