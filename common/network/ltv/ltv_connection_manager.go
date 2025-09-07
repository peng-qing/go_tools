package ltv

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/peng-qing/go_tools/common/network"
)

var (
	// 断言 检查是否实现 network.IConnectionManager 接口
	_ network.IConnectionManager = (*LTVConnectionManager)(nil)
)

// LTVConnectionManager 连接管理器
type LTVConnectionManager struct {
	connIndex int64
	allConn   sync.Map // map[uint64]network.IConnection 所有连接
}

// ===============================================================================
// 实现 network.IConnectionManager 接口
// ===============================================================================

// Add 添加连接
func (ltv *LTVConnectionManager) Add(conn network.IConnection) {
	if _, ok := ltv.allConn.Load(conn.GetConnectionID()); !ok {
		atomic.AddInt64(&ltv.connIndex, 1)
		ltv.allConn.Store(conn.GetConnectionID(), conn)
	}
}

// Remove 删除连接
func (ltv *LTVConnectionManager) Remove(conn network.IConnection) {
	if _, ok := ltv.allConn.Load(conn.GetConnectionID()); ok {
		atomic.AddInt64(&ltv.connIndex, -1)
		ltv.allConn.Delete(conn.GetConnectionID())
	}
}

// RemoveByConnectionID 根据连接ID删除连接
func (ltv *LTVConnectionManager) RemoveByConnectionID(connID uint64) {
	if _, ok := ltv.allConn.Load(connID); ok {
		atomic.AddInt64(&ltv.connIndex, -1)
		ltv.allConn.Delete(connID)
	}
}

// Get 根据连接ID获取连接
func (ltv *LTVConnectionManager) Get(connID uint64) network.IConnection {
	if conn, ok := ltv.allConn.Load(connID); ok {
		return conn.(network.IConnection)
	}
	return nil
}

// Count 连接数量
func (ltv *LTVConnectionManager) Count() int {
	return int(ltv.connIndex)
}

// GetAllConnID 获取所有连接ID
func (ltv *LTVConnectionManager) GetAllConnID() []uint64 {
	allConnIds := make([]uint64, ltv.Count())
	ltv.allConn.Range(func(key, value any) bool {
		allConnIds = append(allConnIds, key.(uint64))
		return true
	})
	return allConnIds
}

// Range 遍历
func (ltv *LTVConnectionManager) Range(fn func(connId uint64, conn network.IConnection) error) error {
	var rangeErr error
	ltv.allConn.Range(func(key, value any) bool {
		if conn, ok := value.(network.IConnection); !ok {
			if err := fn(key.(uint64), conn); err != nil {
				rangeErr = errors.Join(err)
			}
		}
		return true
	})
	return rangeErr
}

// ===============================================================================
// 实例化方法
// ===============================================================================

// NewLTVConnectionManager 创建一个连接管理器
func NewLTVConnectionManager() *LTVConnectionManager {
	return &LTVConnectionManager{allConn: sync.Map{}}
}
