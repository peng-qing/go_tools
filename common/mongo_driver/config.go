package mongo_driver

// MongoDriverConfig 单机配置
type MongoDriverConfig struct {
	Name           string `json:"name" yaml:"name" toml:"name" xml:"name"`                                             // 数据库名称
	Url            string `json:"url" yaml:"url" toml:"url" xml:"url"`                                                 // Mongo地址
	User           string `json:"user" yaml:"user" toml:"user" xml:"user"`                                             // 用户名
	Password       string `json:"password" yaml:"password" toml:"password" xml:"password"`                             // 密码
	ConnectTimeout int64  `json:"connect_timeout" yaml:"connect_timeout" toml:"connect_timeout" xml:"connect_timeout"` // 连接超时时间 单位：秒
	MaxPoolSize    uint64 `json:"max_pool_size" yaml:"max_pool_size" toml:"max_pool_size" xml:"max_pool_size"`         // 最大连接数
	MinPoolSize    uint64 `json:"min_pool_size" yaml:"min_pool_size" toml:"min_pool_size" xml:"min_pool_size"`         // 最小连接数
	AuthMechanism  string `json:"auth_mechanism" yaml:"auth_mechanism" toml:"auth_mechanism" xml:"auth_mechanism"`     // 认证方式
}

// MongoClusterDriverConfig 集群配置
type MongoClusterDriverConfig struct {
	Nodes map[string]*MongoDriverConfig `json:"nodes" yaml:"nodes" toml:"nodes" xml:"nodes"` // 节点配置
}
