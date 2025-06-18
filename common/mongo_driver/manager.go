package mongo_driver

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	DefaultMaxPoolSize    = 10 // 默认连接池最大连接数
	DefaultMinPoolSize    = 5  // 默认连接池最小连接数
	DefaultConnectTimeout = 10 // 默认连接超时时间 单位：秒
)

var ErrInitInvalidConfig = errors.New("invalid mongo config")

// MongoDBManager mongoDB 管理器
type MongoDBManager struct {
	name   string        // 节点名称
	client *mongo.Client // mongoDB 客户端
}

func NewMongoDBManager(name string) *MongoDBManager {
	return &MongoDBManager{
		name: name,
	}
}

// InitConfiguration 初始化配置
func (m *MongoDBManager) InitConfiguration(dbConfig *MongoDriverConfig) error {
	if dbConfig == nil {
		return ErrInitInvalidConfig
	}
	bsonOptions := &options.BSONOptions{
		NilSliceAsEmpty: true, // nil Slice 的值作为空值处理
		NilMapAsEmpty:   true, // nil Map 的值作为空值处理
	}
	authOptions := options.Credential{
		AuthMechanism: dbConfig.AuthMechanism,
		AuthSource:    "admin",
		Username:      dbConfig.User,
		Password:      dbConfig.Password,
	}
	maxPoolSize := dbConfig.MaxPoolSize
	if maxPoolSize == 0 {
		maxPoolSize = DefaultMaxPoolSize
	}
	minPoolSize := dbConfig.MinPoolSize
	if minPoolSize == 0 {
		minPoolSize = DefaultMinPoolSize
	}
	connectTimeout := dbConfig.ConnectTimeout
	if connectTimeout == 0 {
		connectTimeout = DefaultConnectTimeout
	}
	clientOptions := options.Client().ApplyURI(dbConfig.Url).
		SetBSONOptions(bsonOptions).
		SetAuth(authOptions).
		SetMaxPoolSize(maxPoolSize).
		SetMinPoolSize(minPoolSize).
		SetConnectTimeout(time.Duration(connectTimeout) * time.Second)

	client, err := mongo.Connect(clientOptions)
	if err != nil {
		return err
	}
	m.client = client
	m.name = dbConfig.Name

	return nil
}

// Name 获取节点名称
func (m *MongoDBManager) Name() string {
	return m.name
}

// Client 获取 mongoDB 客户端
func (m *MongoDBManager) Client() *mongo.Client {
	return m.client
}

// Database 获取数据库
func (m *MongoDBManager) Database(dbName string) *mongo.Database {
	return m.client.Database(dbName)
}

// ListDatabases 列出所有数据库
func (m *MongoDBManager) ListDatabases(ctx context.Context, filter Filter, opts ...ListDatabasesOptions) (ListDatabasesResult, error) {
	return m.client.ListDatabases(ctx, filter, opts...)
}

// ListDatabaseNames 列出所有数据库名称
func (m *MongoDBManager) ListDatabaseNames(ctx context.Context, filter Filter, opts ...ListDatabasesOptions) ([]string, error) {
	return m.client.ListDatabaseNames(ctx, filter, opts...)
}

// MongoClusterManager mongoDB 集群管理器
type MongoClusterManager struct {
	allNodes sync.Map
}

// NewMongoClusterManager 创建 mongoDB 集群管理器
func NewMongoClusterManager() *MongoClusterManager {
	return &MongoClusterManager{}
}

// GetMongoDBManager 获取 mongoDB 管理器
func (m *MongoClusterManager) GetMongoDBManager(nodeName string) *MongoDBManager {
	if node, ok := m.allNodes.Load(nodeName); ok {
		return node.(*MongoDBManager)
	}
	return nil
}

// AddNodeMongoDBManager 添加 mongoDB 管理器
func (m *MongoClusterManager) AddNodeMongoDBManager(nodeName string, mongoDBManager *MongoDBManager) {
	m.allNodes.Store(nodeName, mongoDBManager)
}

// Destroy 销毁
func (m *MongoClusterManager) Destroy(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var finalErr error

	m.allNodes.Range(func(key, value interface{}) bool {
		if mongoDBManager, ok := value.(*MongoDBManager); ok {
			if err := mongoDBManager.Client().Disconnect(ctx); err != nil {
				finalErr = errors.Join(err)
			}
		}
		return true
	})
	return finalErr
}
