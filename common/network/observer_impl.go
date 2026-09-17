package network

import (
	"net"
)

var (
	_ Observer = (*NopObserver)(nil)
)

// NopObserver 是 Observer 的无副作用空实现 避免频繁判断nil
type NopObserver struct{}

func (NopObserver) OnOpen(id uint64, local, remote net.Addr) {}
func (NopObserver) OnRead(id uint64, bytes int)              {}
func (NopObserver) OnWrite(id uint64, bytes int)             {}
func (NopObserver) OnDecodeError(id uint64, err error)       {}
func (NopObserver) OnReceiverError(id uint64, err error)     {}
func (NopObserver) OnClose(id uint64, err error)             {}
