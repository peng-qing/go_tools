package network

import "errors"

var (
	ErrSendQueueFull       = errors.New("network: send queue is full")                // 发送队列已满
	ErrHeartbeatTimeout    = errors.New("network: heartbeat timeout")                 // 心跳超时
	ErrSessionIdExists     = errors.New("network: register session id already exist") // session id 已经注册
	ErrSessionLimitReached = errors.New("network: session limit reached")             // session 数量达到限制
)
