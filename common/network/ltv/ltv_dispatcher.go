package ltv

import (
	"log/slog"
	"time"

	"go_tools/common/container"
	"go_tools/common/network"
)

// LTVDispatcher 消息分发器
type LTVDispatcher struct {
	allHandler     map[uint32]network.IHandler // 命令集合
	maxExecuteTime int64                       // 命令执行超时时间
	idBegin        uint32                      // 命令ID开始
	idEnd          uint32                      // 命令ID结束
	idPause        *container.Set[uint32]      // 暂停的命令ID
}

func (ltv *LTVDispatcher) Register(id uint32, handler network.IHandler) {
	slog.Debug("[LTVDispatcher] register handler", "id", id)
	if _, ok := ltv.allHandler[id]; ok {
		panic(ErrRepeatRegisterHandler)
		return
	}
	if id < ltv.idBegin || id > ltv.idEnd {
		panic(ErrInvalidCommandID)
		return
	}
	ltv.allHandler[id] = handler
}

func (ltv *LTVDispatcher) DispatchMsg(packet network.IPacket) {
	defer func() {
		if err := recover(); err != nil {
			slog.Error("[LTVDispatcher] DispatchMsg panic", "err", err)
		}
	}()
	header, ok := packet.GetHeader().(*LTVHeader)
	if !ok {
		slog.Error("[LTVDispatcher] DispatchMsg packet get invalid header", "packet", packet)
		return
	}
	// invalid
	if header.Type < ltv.idBegin || header.Type > ltv.idEnd {
		slog.Error("[LTVDispatcher] DispatchMsg get invalid command id", "packet", packet)
		return
	}
	// is paused
	if ltv.idPause.Contains(header.Type) {
		slog.Warn("[LTVDispatcher] DispatchMsg get pause command", "header", header)
		return
	}
	// handler
	handler, exist := ltv.allHandler[header.Type]
	if !exist {
		slog.Error("[LTVDispatcher] DispatchMsg not find handler", "header", header)
		return
	}
	// 开始时间
	startAt := time.Now().UnixNano()
	handler.Execute(handler, packet)
	// 超时处理
	if time.Now().UnixNano()-startAt > ltv.maxExecuteTime {
		slog.Warn("[LTVDispatcher] DispatchMsg handler execute timeout", "header", header)
	}
}

func (ltv *LTVDispatcher) Pause(id uint32) {
	slog.Info("[LTVDispatcher] Pause", "id", id)
	if ltv.idPause.Contains(id) {
		return
	}
	ltv.idPause.Add(id)
}

func (ltv *LTVDispatcher) Resume(id uint32) {
	slog.Info("[LTVDispatcher] Resume", "id", id)
	if !ltv.idPause.Contains(id) {
		return
	}
	ltv.idPause.Remove(id)
}

// NewLTVDispatcher 实例化
func NewLTVDispatcher(idBegin, idEnd uint32, maxExecuteTime int64) *LTVDispatcher {
	return &LTVDispatcher{
		allHandler:     make(map[uint32]network.IHandler),
		maxExecuteTime: maxExecuteTime,
		idBegin:        idBegin,
		idEnd:          idEnd,
		idPause:        container.NewSet[uint32](),
	}
}
