package ltv

type NetMode int

const (
	NetMode_Default   NetMode = iota // 默认
	NetMode_Tcp               = 1    // tcp
	NetMode_Websocket         = 2    // websocket
)

type NetSide int

const (
	NodeSide_Invalid = iota // 无效
	NodeSide_Server         // 服务端
	NodeSide_Client         // 客户端
)
