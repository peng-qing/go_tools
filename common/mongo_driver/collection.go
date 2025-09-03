package mongo_driver

import (
	"context"
	"errors"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var (
	ErrFindFilterNil = errors.New("find filter is nil")
)

// 从mongo复制过来的错误 arguments 错误
var (
	// ErrNilDocument 插入数据为空
	ErrNilDocument = mongo.ErrNilDocument
)

// 从mongo复制过来的错误 查询结果 错误
var (
	// ErrNoDocuments 查询结果为空
	ErrNoDocuments = mongo.ErrNoDocuments
)

// 从mongo复制过来的
var (
	// NilObjectID 空ObjectID
	NilObjectID = bson.NilObjectID
	// ErrInvalidHex 无效的ObjectID
	ErrInvalidHex = bson.ErrInvalidHex
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
func (cb *CollectionBase[T]) ToObjectID(_id string) (bson.ObjectID, error) {
	objectId, err := bson.ObjectIDFromHex(_id)
	if err != nil {
		return NilObjectID, err
	}
	return objectId, nil
}

// ObjectIDToHex 获取ObjectID的Hex
func (cb *CollectionBase[T]) ObjectIDToHex(objectId bson.ObjectID) string {
	return objectId.Hex()
}

// InsertOne 插入数据
func (cb *CollectionBase[T]) InsertOne(ctx context.Context, document *T, opts ...InsertOneOptions) (*InsertOneResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if document == nil {
		return nil, ErrNilDocument
	}
	return cb.Collection.InsertOne(ctx, document, opts...)
}

// InsertMany 批量插入数据
func (cb *CollectionBase[T]) InsertMany(ctx context.Context, documents []*T, opts ...InsertManyOptions) (*InsertManyResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(documents) <= 0 {
		return nil, ErrNilDocument
	}
	return cb.Collection.InsertMany(ctx, documents, opts...)
}

// FindOne 查询单条数据 查询不到返回 ErrNoDocuments
func (cb *CollectionBase[T]) FindOne(ctx context.Context, filter Filter, opts ...FindOneOptions) (*T, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(filter) <= 0 {
		return nil, ErrFindFilterNil
	}
	var (
		result T
		err    error
	)
	singleResult := cb.Collection.FindOne(ctx, filter, opts...)
	if err = singleResult.Err(); err != nil {
		return nil, err
	}
	if err = singleResult.Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// FineCursor Find 查询多条数据 获取结果游标
func (cb *CollectionBase[T]) FineCursor(ctx context.Context, filter Filter, opts ...FindOptions) (*mongo.Cursor, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(filter) <= 0 {
		return nil, ErrFindFilterNil
	}

	return cb.Collection.Find(ctx, filter, opts...)
}

// FindAll 批量查询数据
func (cb *CollectionBase[T]) FindAll(ctx context.Context, filter Filter, opts ...FindOptions) ([]*T, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(filter) <= 0 {
		return nil, ErrFindFilterNil
	}
	cursor, err := cb.FineCursor(ctx, filter, opts...)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	if cursor.Err() != nil {
		return nil, cursor.Err()
	}

	var (
		results = make([]*T, 0)
	)

	for cursor.Next(ctx) {
		var item T
		if err = cursor.Decode(&item); err != nil {
			return nil, err
		}
		results = append(results, &item)
	}

	// 获取结果为空
	//if len(results) <= 0 {
	//	return nil, ErrNoDocuments
	//}

	return results, nil
}

// UpdateOne 更新单条数据
func (cb *CollectionBase[T]) UpdateOne(ctx context.Context, filter Filter, update Operator,
	opts ...UpdateOneOptions) (*UpdateResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(filter) <= 0 {
		return nil, ErrFindFilterNil
	}
	if len(update) <= 0 {
		return nil, ErrNoDocuments
	}

	return cb.Collection.UpdateOne(ctx, filter, update, opts...)
}

// UpdateOneByID 更新数据
func (cb *CollectionBase[T]) UpdateOneByID(ctx context.Context, _id string, updates Operator, opts ...UpdateOneOptions) (*UpdateResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	objectId, err := cb.ToObjectID(_id)
	if err != nil {
		return nil, err
	}
	filter := NewFilterBuilder().EQ("_id", objectId).Build()
	return cb.UpdateOne(ctx, filter, updates, opts...)
}

// UpdateMany 批量更新数据
func (cb *CollectionBase[T]) UpdateMany(ctx context.Context, filter Filter, updates Operator, opts ...UpdateManyOptions) (*UpdateResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(filter) <= 0 {
		return nil, ErrFindFilterNil
	}
	if len(updates) <= 0 {
		return nil, ErrNoDocuments
	}

	return cb.Collection.UpdateMany(ctx, filter, updates, opts...)
}

// DeleteOne 删除单条数据
func (cb *CollectionBase[T]) DeleteOne(ctx context.Context, filter Filter, opts ...DeleteOneOptions) (*DeleteResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(filter) <= 0 {
		return nil, ErrFindFilterNil
	}

	return cb.Collection.DeleteOne(ctx, filter, opts...)
}

// DeleteOneByID 删除数据
func (cb *CollectionBase[T]) DeleteOneByID(ctx context.Context, _id string, opts ...DeleteOneOptions) (*DeleteResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	objectId, err := cb.ToObjectID(_id)
	if err != nil {
		return nil, err
	}
	filter := NewFilterBuilder().EQ("_id", objectId).Build()
	return cb.DeleteOne(ctx, filter, opts...)
}

// DeleteMany 批量删除数据
func (cb *CollectionBase[T]) DeleteMany(ctx context.Context, filter Filter, opts ...DeleteManyOptions) (*DeleteResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(filter) <= 0 {
		return nil, ErrFindFilterNil
	}

	return cb.Collection.DeleteMany(ctx, filter, opts...)
}

// Count 统计数据
func (cb *CollectionBase[T]) Count(ctx context.Context, filter Filter, opts ...CountOptions) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	result, err := cb.Collection.CountDocuments(ctx, filter, opts...)
	if err != nil {
		return 0, err
	}
	return result, nil
}

func (cb *CollectionBase[T]) EstimatedDocumentCount(ctx context.Context, opts ...EstimatedDocumentCountOptions) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := cb.Collection.EstimatedDocumentCount(ctx)
	if err != nil {
		return 0, err
	}
	return result, nil
}
