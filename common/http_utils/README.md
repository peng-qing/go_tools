# http_utils

对标准库 `net/http` 的轻量封装，提供独立会话、请求选项、Cookie、Hook 和响应解析。

## 主要类型

| 类型/函数 | 说明 |
| --- | --- |
| `Session` | 持有 `http.Client`、Cookie Jar 和请求/响应 Hook |
| `HttpHeader` | 查询参数、请求体、认证、代理、超时和传输选项 |
| `Response` | 包装 `http.Response`，缓存 `Text` 与 `Bytes` |
| `FileUploadMeta` | 内存或磁盘文件的上传描述 |
| `Get/Post/... ` | 使用包级默认 Session 的快捷方法 |

支持 GET、POST、DELETE、PUT、PATCH、HEAD 和 OPTIONS。

## JSON 请求

```go
package main

import (
    "fmt"
    "log"

    "github.com/peng-qing/go_tools/common/http_utils"
)

func main() {
    session := http_utils.NewSession()

    _, response, err := session.Post("https://httpbin.org/post", &http_utils.HttpHeader{
        Params:  map[string]string{"source": "go_tools"},
        Headers: map[string]string{"Accept": "application/json"},
        JSON:    map[string]any{"name": "demo"},
        Timeout: 10,
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(response.StatusCode)
    fmt.Println(response.Text)
}
```

## 文件上传

```go
options := &http_utils.HttpHeader{
    Files: map[string]any{
        "description": "avatar",
        "file": http_utils.FileFromPath("./avatar.png").
            SetMIME("image/png"),
    },
}
_, response, err := http_utils.Post(uploadURL, options)
```

也可使用 `http_utils.File("name.txt", data)` 上传内存数据。

## 请求选项

- `Params`：URL 查询参数。
- `Data`：`application/x-www-form-urlencoded` 表单。
- `JSON`：JSON 请求体。
- `Files`：`multipart/form-data` 字段与文件。
- `RowData`：原始字符串请求体。
- `Headers`、`Cookies`、`Auth`：请求头、Cookie 和 Basic Auth。
- `Proxy`、`Timeout`、`Chunked`：代理、秒级超时和分块传输。
- `DisableKeepalives`、`DisableCompression`、`SkipVerifyTLS`：传输层选项。

`Data`、`JSON`、`Files` 和 `RowData` 互斥，一次请求只能设置一种请求体。

## Response

`Response.JSON` 将响应体反序列化到目标对象；`SetEncoding` 转换文本编码；`SaveFile` 将原始响应字节保存到文件。

## Hook 与并发

使用 `AddRequestHooks` 和 `AddResponseHooks` 注册 Hook，使用对应的 `Reset...` 方法清空。每个 `Session` 内部串行化请求；需要并行请求时建议为不同工作流创建独立 Session。

`SkipVerifyTLS` 会关闭证书校验，只应在受控测试环境使用。
