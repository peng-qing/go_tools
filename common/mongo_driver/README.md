# mongo_driver

基于 MongoDB Go Driver v2 的连接管理和泛型 CRUD 封装。

## 组成

| 文件 | 说明 |
| --- | --- |
| `config.go` | 单节点与多节点配置 |
| `manager.go` | 节点连接和集群生命周期 |
| `collection.go` | `CollectionBase[T]` 泛型 CRUD |
| `filter.go` | MongoDB 查询过滤器构建器 |
| `operator.go` | 更新操作构建器 |
| `projection.go` | 字段投影构建器 |
| `wrapper.go` | BSON、结果和选项类型别名，以及包级管理入口 |

## 初始化

```go
package main

import (
    "context"
    "log"

    mongo "github.com/peng-qing/go_tools/common/mongo_driver"
)

func main() {
    err := mongo.InitConfiguration(&mongo.MongoClusterDriverConfig{
        Nodes: map[string]*mongo.MongoDriverConfig{
            "primary": {
                Name:           "primary",
                Url:            "mongodb://127.0.0.1:27017",
                User:           "app",
                Password:       "secret",
                ConnectTimeout: 10,
                MaxPoolSize:    20,
                MinPoolSize:    5,
            },
        },
    })
    if err != nil {
        log.Fatal(err)
    }
    defer mongo.Destroy(context.Background())
}
```

连接池字段为 `0` 时使用默认值：最大连接数 10、最小连接数 5、连接超时 10 秒。认证源固定为 `admin`。

## 泛型集合

```go
type User struct {
    ID     any    `bson:"_id,omitempty"`
    Name   string `bson:"name"`
    Status string `bson:"status"`
    Score  int    `bson:"score"`
}

users, err := mongo.CreateCollectionBase[User]("primary", "app", "users")
if err != nil {
    return err
}

filter := mongo.NewFilterBuilder().
    EQ("status", "active").
    GTE("score", 60).
    Build()

rows, err := users.FindAll(ctx, filter)
```

`CollectionBase[T]` 提供 InsertOne/InsertMany、FindOne/FindAll、UpdateOne/UpdateMany、DeleteOne/DeleteMany、Count 和 EstimatedDocumentCount 等操作，并提供 ObjectID 与十六进制字符串转换。

## 更新与投影

```go
update := mongo.NewOperatorBuilder().
    Set(mongo.E{Key: "verified", Value: true}).
    Increment(mongo.E{Key: "login_count", Value: 1}).
    Build()

projection := mongo.NewProjectionBuilder().
    Fields("name", "status").
    Excludes("_id").
    Build()
```

过滤器支持比较、区间、集合、元素、存在性和逻辑组合；更新器支持 Set、Unset、Increment、Rename、Push、Pull、AddToSet、CurrentDate 等操作。

## 生命周期

应用退出时调用 `Destroy` 断开全部节点。创建集合前必须已初始化对应的命名节点；密码等敏感配置不要提交到仓库。
