# encode_utils

基于 `golang.org/x/text/encoding` 创建文本编码器和解码器。

## 支持的编码

| 常量 | 编码 |
| --- | --- |
| `EncodingUTF8` | UTF-8 |
| `EncodingUTF8BOM` | 带 BOM 的 UTF-8 |
| `EncodingGBK` | GBK |
| `EncodingGB18030` | GB18030 |
| `EncodingHZGB2312` | HZ-GB2312 |

## 示例

```go
package main

import (
    "fmt"

    "github.com/peng-qing/go_tools/common/encode_utils"
)

func main() {
    decoder := encode_utils.NewDecoder(encode_utils.EncodingGBK)
    if decoder == nil {
        panic("unsupported encoding")
    }

    text, err := decoder.String(string([]byte{0xc4, 0xe3, 0xba, 0xc3}))
    if err != nil {
        panic(err)
    }
    fmt.Println(text)
}
```

`NewEncoder` 和 `NewDecoder` 只识别表中的精确常量值。不支持的名称会返回 `nil`，调用方法前应先检查。
