package ltv

// NetMode 网络模式
type NetMode int

const (
	NetMode_Default   NetMode = iota // 默认 server默认同时监听Tcp和Websocket 客户端默认监听TCP
	NetMode_Tcp               = 1    // tcp
	NetMode_Websocket         = 2    // websocket
)

// NetSide 网络端
type NetSide int

const (
	NodeSide_Invalid = iota // 无效
	NodeSide_Server         // 服务端
	NodeSide_Client         // 客户端
)
