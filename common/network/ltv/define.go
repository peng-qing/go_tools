package ltv

const (
	Tcp       = 1
	Websocket = 2
)

type NetSide int

const (
	NodeSide_Invalid = iota
	NodeSide_Server
	NodeSide_Client
)
