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

// SingleResult Alias for mongo.SingleResult
type SingleResult = mongo.SingleResult

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

// InsertOneOptions Alias for options.InsertOneOptions
type InsertOneOptions = options.Lister[options.InsertOneOptions]

// InsertOneResult Alias for mongo.InsertOneResult
type InsertOneResult = mongo.InsertOneResult

// NewInsertOneOptions 创建一个 InsertOneOptions
var NewInsertOneOptions = options.InsertOne

// InsertManyOptions Alias for options.InsertManyOptions
type InsertManyOptions = options.Lister[options.InsertManyOptions]

// InsertManyResult Alias for mongo.InsertManyResult
type InsertManyResult = mongo.InsertManyResult

// NewInsertManyOptions 创建一个 InsertManyOptions
var NewInsertManyOptions = options.InsertMany

// UpdateResult Alias for mongo.UpdateResult
type UpdateResult = mongo.UpdateResult

// UpdateOneOptions Alias for options.UpdateOneOptions
type UpdateOneOptions = options.Lister[options.UpdateOneOptions]

// NewUpdateOneOptions 创建一个 UpdateOneOptions
var NewUpdateOneOptions = options.UpdateOne

// UpdateManyOptions Alias for options.UpdateManyOptions
type UpdateManyOptions = options.Lister[options.UpdateManyOptions]

// NewUpdateManyOptions 创建一个 NewUpdateManyOptions
var NewUpdateManyOptions = options.UpdateMany

// DeleteOneOptions Alias for options.DeleteOneOptions
type DeleteOneOptions = options.Lister[options.DeleteOneOptions]

// DeleteResult Alias for mongo.DeleteResult
type DeleteResult = mongo.DeleteResult

// NewDeleteOneOptions 创建一个 DeleteOneOptions
var NewDeleteOneOptions = options.DeleteOne

// DeleteManyOptions Alias for options.DeleteManyOptions
type DeleteManyOptions = options.Lister[options.DeleteManyOptions]

// NewDeleteManyOptions 创建一个 DeleteManyOptions
var NewDeleteManyOptions = options.DeleteMany

// CountOptions Alias for options.CountOptions
type CountOptions = options.Lister[options.CountOptions]

// NewCountOptions 创建一个 CountOptions
var NewCountOptions = options.Count

// EstimatedDocumentCountOptions Alias for options.EstimatedDocumentCountOptions
type EstimatedDocumentCountOptions = options.Lister[options.EstimatedDocumentCountOptions]

// NewEstimatedDocumentCountOptions 创建一个 EstimatedDocumentCountOptions
var NewEstimatedDocumentCountOptions = options.EstimatedDocumentCount

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
