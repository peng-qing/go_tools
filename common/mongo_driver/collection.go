package mongo_driver

import (
	"errors"
	"go.mongodb.org/mongo-driver/v2/bson"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// CollectionBase 集合基类
type CollectionBase[T any] struct {
	NodeName   string            // 节点名
	DBName     string            // 数据库名
	TableName  string            // 表名
	Collection *mongo.Collection // 集合
}

// CreateCollectionBase 创建集合
func CreateCollectionBase[T any](nodeName, dbName, tableName string) (*CollectionBase[T], error) {
	mongoDBManager := GetMongoDBManager(nodeName)
	if mongoDBManager == nil {
		return nil, errors.New("mongo manager not exist")
	}
	collection := mongoDBManager.Database(dbName).Collection(tableName)

	return &CollectionBase[T]{
		DBName:     dbName,
		TableName:  tableName,
		Collection: collection,
	}, nil
}

// ToObjectID 转换为ObjectID
func (cb *CollectionBase[T]) ToObjectID(_id string) bson.ObjectID {
	objectId, err := bson.ObjectIDFromHex(_id)
	if err != nil {
		return bson.NilObjectID
	}
	return objectId
}

// ObjectIDToHex 获取ObjectID的Hex
func (cb *CollectionBase[T]) ObjectIDToHex(objectId bson.ObjectID) string {
	return objectId.Hex()
}
