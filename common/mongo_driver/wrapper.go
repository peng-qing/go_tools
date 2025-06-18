package mongo_driver

import (
	"context"
	"errors"
	"sync/atomic"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// A Alias for bson.A 文档的数组表示
type A = bson.A

// D Alias for bson.D 文档的有序表示
type D = bson.D

// M Alias for bson.M 文档的映射表示
type M = bson.M

// E Alias for bson.E 文档的键值对表示
type E = bson.E

// ListDatabasesOptions Alias for options.ListDatabasesOptions
type ListDatabasesOptions = options.Lister[options.ListDatabasesOptions]

// ListDatabasesResult Alias for mongo.ListDatabasesResult
type ListDatabasesResult = mongo.ListDatabasesResult

// FindOptions Alias for options.FindOptions
type FindOptions = options.Lister[options.FindOptions]

// NewFindOptions 创建一个 FindOptions
var NewFindOptions = options.Find

// FindOneOptions Alias for options.FindOneOptions
type FindOneOptions = options.Lister[options.FindOneOptions]

// NewFindOneOptions 创建一个 FindOneOptions
var NewFindOneOptions = options.FindOne

// FindOneAndUpdateOptions Alias for options.FindOneAndUpdateOptions
type FindOneAndUpdateOptions = options.Lister[options.FindOneAndUpdateOptions]

// NewFindOneAndUpdateOptions 创建一个 FindOneAndUpdateOptions
var NewFindOneAndUpdateOptions = options.FindOneAndUpdate

// FindOneAndDeleteOptions Alias for options.FindOneAndDeleteOptions
type FindOneAndDeleteOptions = options.Lister[options.FindOneAndDeleteOptions]

// NewFindOneAndDeleteOptions 创建一个 FindOneAndDeleteOptions
var NewFindOneAndDeleteOptions = options.FindOneAndDelete

// FindOneAndReplaceOptions Alias for options.FindOneAndReplaceOptions
type FindOneAndReplaceOptions = options.Lister[options.FindOneAndReplaceOptions]

// NewFindOneAndReplaceOptions 创建一个 FindOneAndReplaceOptions
var NewFindOneAndReplaceOptions = options.FindOneAndReplace

var (
	// gClusterMongoDBManager 集群管理器
	gClusterMongoDBManager atomic.Pointer[MongoClusterManager]
)

var (
	ErrMongoDriverAlreadyInit = errors.New("mongo driver already init")
)

// InitConfiguration 初始化配置
func InitConfiguration(dbConf *MongoClusterDriverConfig) error {
	if dbConf == nil {
		return ErrInitInvalidConfig
	}
	if gClusterMongoDBManager.Load() != nil {
		return ErrMongoDriverAlreadyInit
	}
	mongoClusterManager := NewMongoClusterManager()
	for nodeName, dbConfig := range dbConf.Nodes {
		if dbConfig == nil {
			return ErrInitInvalidConfig
		}
		mongoDBManager := NewMongoDBManager(nodeName)
		if err := mongoDBManager.InitConfiguration(dbConfig); err != nil {
			return err
		}
		mongoClusterManager.AddNodeMongoDBManager(nodeName, mongoDBManager)
	}

	// 到这里算初始化成功了
	gClusterMongoDBManager.Store(mongoClusterManager)
	return nil
}

// GetMongoDBManager 获取对应节点的 MongoDBManager
func GetMongoDBManager(nodeName string) *MongoDBManager {
	mongoClusterManager := gClusterMongoDBManager.Load()
	if mongoClusterManager == nil {
		return nil
	}
	return mongoClusterManager.GetMongoDBManager(nodeName)
}

// Destroy 销毁释放db连接
func Destroy(ctx context.Context) error {
	mongoClusterManager := gClusterMongoDBManager.Load()
	if mongoClusterManager == nil {
		return nil
	}
	return mongoClusterManager.Destroy(ctx)
}
