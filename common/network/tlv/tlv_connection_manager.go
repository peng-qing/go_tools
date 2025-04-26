package tlv

import (
	"errors"
	"go_tools/common/network"
	"iter"
	"sync"
	"sync/atomic"
)

type TLVConnectionManager struct {
	connIndex int64
	allConn   sync.Map // map[uint64]network.IConnection 所有连接
}

// Add 添加连接
func (tlv *TLVConnectionManager) Add(conn network.IConnection) {
	if _, ok := tlv.allConn.Load(conn.GetConnectionID()); !ok {
		atomic.AddInt64(&tlv.connIndex, 1)
		tlv.allConn.Store(conn.GetConnectionID(), conn)
	}
}

// Remove 删除连接
func (tlv *TLVConnectionManager) Remove(conn network.IConnection) {
	if _, ok := tlv.allConn.Load(conn.GetConnectionID()); ok {
		atomic.AddInt64(&tlv.connIndex, -1)
		tlv.allConn.Delete(conn.GetConnectionID())
	}
}

// RemoveByConnectionID 根据连接ID删除连接
func (tlv *TLVConnectionManager) RemoveByConnectionID(connID uint64) {
	if _, ok := tlv.allConn.Load(connID); ok {
		atomic.AddInt64(&tlv.connIndex, -1)
		tlv.allConn.Delete(connID)
	}
}

// Get 根据连接ID获取连接
func (tlv *TLVConnectionManager) Get(connID uint64) network.IConnection {
	if conn, ok := tlv.allConn.Load(connID); ok {
		return conn.(network.IConnection)
	}
	return nil
}

// Count 连接数量
func (tlv *TLVConnectionManager) Count() int {
	return int(tlv.connIndex)
}

// GetAllConnID 获取所有连接ID
func (tlv *TLVConnectionManager) GetAllConnID() []uint64 {
	allConnIds := make([]uint64, tlv.Count())
	tlv.allConn.Range(func(key, value any) bool {
		allConnIds = append(allConnIds, key.(uint64))
		return true
	})
	return allConnIds
}

// Iter 迭代器
func (tlv *TLVConnectionManager) Iter() iter.Seq[network.IConnection] {
	return func(yield func(network.IConnection) bool) {
		tlv.allConn.Range(func(_, value any) bool {
			if conn, ok := value.(network.IConnection); ok {
				return yield(conn)
			}
			return true
		})
	}
}

// Range 遍历
func (tlv *TLVConnectionManager) Range(fn func(connId uint64, conn network.IConnection) error) error {
	var rangeErr error
	tlv.allConn.Range(func(key, value any) bool {
		if conn, ok := value.(network.IConnection); !ok {
			if err := fn(key.(uint64), conn); err != nil {
				errors.Join(err)
			}
		}
		return true
	})
	return rangeErr
}

// NewTLVConnectionManager 创建一个连接管理器
func NewTLVConnectionManager() *TLVConnectionManager {
	return &TLVConnectionManager{allConn: sync.Map{}}
}
