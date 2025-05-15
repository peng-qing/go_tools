package ltv

import (
	"log/slog"
	"sync"
	"time"

	"go_tools/common/container"
	"go_tools/common/network"
)

var (
	// 断言 检查是否实现 network.IDispatcher
	_ network.IDispatcher = (*LTVDispatcher)(nil)
)

// LTVDispatcher 消息分发器
type LTVDispatcher struct {
	allHandler     map[uint32]network.IHandler // 命令集合
	maxExecuteTime int64                       // 命令执行超时时间
	idBegin        uint32                      // 命令ID开始
	idEnd          uint32                      // 命令ID结束
	idPause        *container.Set[uint32]      // 暂停的命令ID
	lock           sync.RWMutex                // 读写锁
}

// ===============================================================================
// 实现 network.IDispatcher 接口
// ===============================================================================

// Register 注册消息处理
func (ltv *LTVDispatcher) Register(id uint32, handler network.IHandler) {
	slog.Debug("[LTVDispatcher] Register handler", "id", id)

	ltv.lock.Lock()
	defer ltv.lock.Unlock()

	if _, ok := ltv.allHandler[id]; ok {
		slog.Error("[LTVDispatcher] Register repeat register handler", "id", id)
		return
	}
	if id < ltv.idBegin || id > ltv.idEnd {
		slog.Error("[LTVDispatcher] Register id out of range", "id", id, "begin", ltv.idBegin, "end", ltv.idEnd)
		return
	}
	ltv.allHandler[id] = handler
}

// DispatchMsg 消息分发
func (ltv *LTVDispatcher) DispatchMsg(dealConn network.IConnection, packet network.IPacket) {
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

	// read lock
	ltv.lock.RLock()
	defer ltv.lock.RUnlock()

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
	handler.Execute(dealConn, packet)
	endAt := time.Now().UnixNano()
	// 超时处理
	if endAt-startAt > ltv.maxExecuteTime {
		slog.Warn("[LTVDispatcher] DispatchMsg handler execute timeout", "header", header)
	}
}

// Pause 暂停消息处理
func (ltv *LTVDispatcher) Pause(id uint32) {
	slog.Info("[LTVDispatcher] Pause", "id", id)
	ltv.lock.Lock()
	defer ltv.lock.Unlock()

	if ltv.idPause.Contains(id) {
		return
	}
	ltv.idPause.Add(id)
}

// Resume 恢复消息处理
func (ltv *LTVDispatcher) Resume(id uint32) {
	slog.Info("[LTVDispatcher] Resume", "id", id)
	ltv.lock.Lock()
	defer ltv.lock.Unlock()

	if !ltv.idPause.Contains(id) {
		return
	}
	ltv.idPause.Remove(id)
}

// ===============================================================================
// 实例化方法
// ===============================================================================

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
