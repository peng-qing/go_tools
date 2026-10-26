package network

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

var (
	_ Connection[any] = (*Conn[any])(nil) // 校验是否实现 Connection 接口
)

var (
	connIdGenerator atomic.Uint64 // 连接id生成器
)

const (
	defaultConnSendQueueSize  = 256  // 默认发送队列容量
	defaultConnReadBufferSize = 4096 // 默认读缓冲容量
)

type Conn[P any] struct {
	id             uint64                                     // 连接标识 进程内唯一
	rwc            Transport                                  // 底层io接口
	codec          Codec[P]                                   // 编解码器
	receiver       Receiver[P]                                // 业务消息接收器
	heartbeat      Heartbeator[P]                             // 应用层心跳器
	onOpen         func(context.Context, Connection[P]) error // 连接初始化回调 在读取消息前调用 可发送初始消息
	onClose        func(Connection[P], error)                 // 连接结束回调 在所有循环退出且底层关闭完成后调用
	connConf       ConnConfig                                 // 连接配置
	ctx            context.Context                            // 连接生命周期
	ctxCancel      context.CancelCauseFunc                    // 取消连接并纪录终止原因
	sendQueue      chan []byte                                // 发送队列
	wg             sync.WaitGroup                             // 等待组
	started        atomic.Int32                               // 防止重复启动
	closeOnce      sync.Once                                  // 保证只关闭一次
	closeErr       error                                      // 关闭错误
	lastActiveTime atomic.Int64                               // 最后活跃时间 (收到数据为准)
}

// newConn 创建连接
func newConn[P any](rwc Transport, deps ConnDependencies[P], connConf ConnConfig) (*Conn[P], error) {
	if rwc == nil {
		return nil, errors.New("network/conn: transport is nil")
	}
	if connConf.SendQueueSize <= 0 {
		connConf.SendQueueSize = defaultConnSendQueueSize
	}
	if connConf.ReadBufferSize <= 0 {
		connConf.ReadBufferSize = defaultConnReadBufferSize
	}
	ctx, ctxCancel := context.WithCancelCause(context.Background())
	return &Conn[P]{
		id:        connIdGenerator.Add(1),
		rwc:       rwc,
		codec:     deps.Codec,
		receiver:  deps.Receiver,
		heartbeat: deps.Heartbeator,
		onOpen:    deps.OnOpen,
		onClose:   deps.OnClose,
		connConf:  connConf,
		ctx:       ctx,
		ctxCancel: ctxCancel,
		sendQueue: make(chan []byte, connConf.SendQueueSize),
	}, nil
}

// ID 返回进程内唯一的连接标识
func (c *Conn[P]) ID() uint64 {
	return c.id
}

// LocalAddr 返回本地地址
func (c *Conn[P]) LocalAddr() net.Addr {
	return c.rwc.LocalAddr()
}

// RemoteAddr 返回远程地址
func (c *Conn[P]) RemoteAddr() net.Addr {
	return c.rwc.RemoteAddr()
}

// Err 返回首个终止原因，尚未终止时返回 nil
func (c *Conn[P]) Err() error {
	return context.Cause(c.ctx)
}

// Send 编码并发送消息 成功表示消息入队 不代表对端成功接收
func (c *Conn[P]) Send(ctx context.Context, packet P) error {
	if ctx == nil {
		ctx = context.Background()
	}
	data, err := c.codec.Encode(packet)
	if err != nil {
		return err
	}
	// 提前过滤终止
	if err := c.Err(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// 等待入队
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.ctx.Done():
		return c.Err()
	case c.sendQueue <- data:
		return nil
	}
}

// TrySend 编码并发送消息 但是不等待队列空位 队列满返回 ErrSendQueueFull。
func (c *Conn[P]) TrySend(packet P) error {
	data, err := c.codec.Encode(packet)
	if err != nil {
		return err
	}
	if err := c.Err(); err != nil {
		return err
	}
	select {
	case <-c.ctx.Done():
		return c.Err()
	case c.sendQueue <- data:
		return nil
	default:
		return ErrSendQueueFull
	}
}

// Close 取消连接并中断网络 I/O，不等待业务回调
// 当前实现不保证发送队列排空
func (c *Conn[P]) Close() error {
	c.closeWithCause(net.ErrClosed)
	return c.closeErr
}

// Run 阻塞允许 重复调用立即返回
func (c *Conn[P]) Run() {
	if !c.started.CompareAndSwap(0, 1) {
		return
	}

	// 先启动写循环 允许OnOpen发送初始消息
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer func() {
			if cause := recover(); cause != nil {
				c.closeWithCause(fmt.Errorf("network/conn: write loop panic: %v\n%s", cause, debug.Stack()))
			}
		}()
		c.writeLoop()
	}()

	if err := c.callOnOpen(); err != nil {
		c.closeWithCause(err)
	}

	// 避免 OnOpen 主动关闭连接
	if c.Err() == nil {
		// 记录活跃时间
		c.lastActiveTime.Store(time.Now().UnixNano())
		// 启动读循环
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			defer func() {
				if cause := recover(); cause != nil {
					c.closeWithCause(fmt.Errorf("network/conn: read loop panic: %v\n%s", cause, debug.Stack()))
				}
			}()
			c.readLoop()
		}()

		// 注入心跳依赖后由conn维护心跳
		if c.heartbeat != nil && c.connConf.HeartbeatInterval > 0 {
			c.wg.Add(1)
			go func() {
				defer c.wg.Done()
				defer func() {
					if cause := recover(); cause != nil {
						c.closeWithCause(fmt.Errorf("network/conn: heartbeat loop panic: %v\n%s", cause, debug.Stack()))
					}
				}()
				c.heartbearLoop()
			}()
		}
	}

	// 等待退出
	c.wg.Wait()

	// 等待可能由其他协程发起的底层 Close 完成
	c.closeWithCause(c.Err())

	// 执行 OnClose
	c.callOnClose()
}

// closeWithCause 关闭并记录原因
func (c *Conn[P]) closeWithCause(cause error) {
	if cause == nil {
		cause = net.ErrClosed
	}
	c.closeOnce.Do(func() {
		if c.ctxCancel != nil {
			c.ctxCancel(cause)
		}
		fmt.Fprintf(os.Stderr, "[Conn] conn is about to be close, connID:%d, cause:%v\n", c.id, cause)
		c.closeErr = c.rwc.Close()
	})
}

// writeLoop 写循环
func (c *Conn[P]) writeLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case data := <-c.sendQueue:
			if c.Err() != nil {
				return
			}
			nBytes, err := c.rwc.Write(data)
			if err != nil {
				c.closeWithCause(err)
				return
			}
			if nBytes != len(data) {
				c.closeWithCause(io.ErrShortWrite)
				return
			}
		}
	}
}

// readLoop 读循环 保留半包并处理粘包
func (c *Conn[P]) readLoop() {
	reader := bufio.NewReaderSize(c.rwc, c.connConf.ReadBufferSize)
	for {
		// 关闭后结束
		if c.Err() != nil {
			return
		}
		packet, err := c.codec.Decode(reader)
		if err != nil {
			c.closeWithCause(err)
			return
		}
		if c.Err() != nil {
			return
		}
		// 收到完整消息更新活跃时间 包括业务消息和心跳消息
		c.lastActiveTime.Store(time.Now().UnixNano())
		if err := c.deliverPacket(packet); err != nil {
			c.closeWithCause(err)
			return
		}
	}
}

// deliverPacket 交付协议结果到给相应的处理器
func (c *Conn[P]) deliverPacket(packet P) (err error) {
	if c.heartbeat != nil {
		kind, seq, ok := c.heartbeat.Inspect(packet)
		if ok {
			switch kind {
			case HeartbeatKind_Ping:
				return c.Send(c.ctx, c.heartbeat.Pong(seq))
			case HeartbeatKind_Pong:
				return nil
			default:
				return fmt.Errorf("network/conn: invalid heartbeat kind %d", kind)
			}
		}
	}
	if c.receiver != nil {
		defer func() {
			if cause := recover(); cause != nil {
				err = fmt.Errorf("network/conn: OnMessage panic: %v\n%s", cause, debug.Stack())
			}
		}()

		c.receiver.OnMessage(c.ctx, c, packet)
	}

	// 默认丢弃
	return nil
}

// heartbearLoop 心跳循环
func (c *Conn[P]) heartbearLoop() {
	ticker := time.NewTicker(c.connConf.HeartbeatInterval)
	defer ticker.Stop()
	seq := uint64(0)
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if c.Err() != nil {
				return
			}
			lastActiveTime := time.Unix(0, c.lastActiveTime.Load())
			timeout := c.connConf.HeartbeatTimeout
			if timeout <= 0 {
				timeout = 3 * c.connConf.HeartbeatInterval
			}
			if time.Since(lastActiveTime) >= timeout {
				c.closeWithCause(ErrHeartbeatTimeout)
				return
			}
			seq++
			// 等待发送队列空位，最多等待一个心跳周期
			ctx, ctxCancel := context.WithTimeout(c.ctx, c.connConf.HeartbeatInterval)
			err := c.Send(ctx, c.heartbeat.Ping(seq))
			ctxCancel()
			if c.Err() != nil {
				return
			}
			if err != nil {
				if ctx.Err() == context.DeadlineExceeded && errors.Is(err, context.DeadlineExceeded) {
					// 本次未能入队 下一轮继续
					continue
				}
				c.closeWithCause(err)
				return
			}
		}
	}
}

// callOnOpen 调用连接初始化回调 将 panic 转为终止原因
func (c *Conn[P]) callOnOpen() (err error) {
	if c.onOpen == nil {
		return nil
	}
	defer func() {
		if cause := recover(); cause != nil {
			err = fmt.Errorf("network/conn: OnOpen panic:%v\n%s", cause, debug.Stack())
		}
	}()

	return c.onOpen(c.ctx, c)
}

// callOnClose 调用连接结束回调 不改变连接的首个终止原因
func (c *Conn[P]) callOnClose() {
	if c.onClose == nil {
		return
	}
	defer func() {
		if cause := recover(); cause != nil {
			fmt.Fprintf(os.Stderr, "[Conn] OnClose panic, connID:%d, err:%v, stack:\n%s", c.id, cause, debug.Stack())
		}
	}()

	c.onClose(c, c.Err())
}
