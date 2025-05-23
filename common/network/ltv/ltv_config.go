package ltv

// LTVServerConfig 服务器配置
type LTVServerConfig struct {
	IP               string               `yaml:"ip" json:"ip"`                                 // IP 服务器IP地址
	Port             int                  `yaml:"port" json:"port"`                             // 端口
	WsPort           int                  `yaml:"ws_port" json:"ws_port"`                       // Websocket 端口
	WsPath           string               `yaml:"ws_path" json:"ws_path"`                       // Websocket 路径
	Mode             int                  `yaml:"mode" json:"mode"`                             // 监听服务 1 TCP 2 WebSocket 0 默认同时监听
	MaxConn          int                  `yaml:"max_conn" json:"max_conn"`                     // 最大连接数
	UsedLittleEndian bool                 `yaml:"used_little_endian" json:"used_little_endian"` // UsedLittleEndian 是否使用小端模式
	TimerQueueSize   int                  `yaml:"timer_queue_size" json:"timer_queue_size"`     // TimerQueueSize 定时器队列大小
	Frequency        int                  `yaml:"frequency" json:"frequency"`                   // Frequency 定时器频率 单位: 毫秒
	Connection       *LTVConnectionConfig `yaml:"connection" json:"connection"`                 // Connection 连接配置
}

// LTVConnectionConfig 连接配置
type LTVConnectionConfig struct {
	Heartbeat     int64 `yaml:"heartbeat" json:"heartbeat"`               // Heartbeat 心跳间隔 单位 毫秒
	MaxHeartbeat  int64 `yaml:"max_heartbeat" json:"max_heartbeat"`       // MaxHeartbeat 最大心跳间隔 单位 毫秒 超过视为超时
	ReadTimeout   int64 `yaml:"read_timeout" json:"read_timeout"`         // ReadTimeout 读取超时时间 单位 毫秒
	WriteTimeout  int64 `yaml:"write_timeout" json:"write_timeout"`       // WriteTimeout 写入超时时间 单位 毫秒
	MaxIOReadSize int   `yaml:"max_io_read_size" json:"max_io_read_size"` // MaxIOReadSize 一次读取的最大字节数 单位byte
	SendQueueSize int   `yaml:"send_queue_size" json:"send_queue_size"`   // SendQueueSize 发送队列大小
}

// LTVClientConfig 客户端配置
type LTVClientConfig struct {
	IP               string               `yaml:"ip" json:"ip"`                                 // IP 服务器IP地址
	Port             int                  `yaml:"port" json:"port"`                             // 端口
	Mode             int                  `yaml:"mode" json:"mode"`                             // 监听服务 1 TCP 2 WebSocket 0 默认监听TCP
	UsedLittleEndian bool                 `yaml:"used_little_endian" json:"used_little_endian"` // UsedLittleEndian 是否使用小端模式
	Connection       *LTVConnectionConfig `yaml:"connection" json:"connection"`                 // Connection 连接配置
}
