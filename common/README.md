# common

`common` 集中存放可独立引用的服务端基础组件。各目录对应一个 Go 包，详细说明见包内 README。

| 目录 | 主要能力 |
| --- | --- |
| [container](container/README.md) | Queue、Set、SortedMap、LRU、分片 LRU、2Q 缓存 |
| [encode_utils](encode_utils/README.md) | UTF-8、GBK、GB18030、HZ-GB2312 编解码 |
| [http_utils](http_utils/README.md) | HTTP Session、请求参数、文件上传、Hook、响应解析 |
| [mongo_driver](mongo_driver/README.md) | MongoDB 节点管理、泛型集合、查询与更新构建器 |
| [network](network/README.md) | 网络组件接口、连接退避和流量统计 |
| [network/ltv](network/ltv/README.md) | LTV 协议的 TCP/WebSocket 客户端与服务端 |
| [options](options/README.md) | 泛型函数式选项接口 |
| [pool](pool/README.md) | 泛型对象池和可复用 Buffer |
| [pool/gpool](pool/gpool/README.md) | 固定工作者池和并发任务执行器 |
| [profile](profile/README.md) | pprof、强制 GC 和堆快照接口 |
| [timer](timer/README.md) | 单次与周期定时任务 |

导入路径统一以 `github.com/peng-qing/go_tools/common/` 开头。
