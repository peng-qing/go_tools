# options

一个最小化的泛型函数式选项抽象，用于在构造对象时应用可组合配置。

## API

```go
type Option[T any] interface {
    Apply(*T)
}

type WrapperOptions[T any] func(*T)
```

`WrapperOptions[T]` 实现了 `Option[T]`，普通函数经过类型转换即可作为选项。

## 示例

```go
package main

import "github.com/peng-qing/go_tools/common/options"

type Server struct {
    Name string
    Port int
}

func WithPort(port int) options.Option[Server] {
    return options.WrapperOptions[Server](func(s *Server) {
        s.Port = port
    })
}

func NewServer(opts ...options.Option[Server]) *Server {
    server := &Server{Name: "default", Port: 8080}
    for _, opt := range opts {
        opt.Apply(server)
    }
    return server
}

func main() {
    _ = NewServer(WithPort(9000))
}
```

网络模块的服务端和客户端构造函数也接受该形式的泛型选项。
