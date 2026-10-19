---
title: gRPC
weight: 2
prev: /docs/protocols/http
next: /docs/protocols/graphql
---

包：`github.com/sao-lang/lania-g/protocol/grpc/v3`

## 0. 装配 Config

```go
grpcadapter.Module(grpcadapter.Config{
	Addr:          ":50051",
	ServerOptions: []grpc.ServerOption{grpc.Creds(creds)},
	// Server:    预配置的 *grpc.Server（优先级高于 ServerOptions）
})
```

## 1. 声明服务与方法

```go
func (s *UserService) Declare(r *grpcadapter.Routes) {
	r.Service("UserService").
		Method("Ping", s.Ping).
		ServerStreamMethod("Watch", s.Watch).
		ClientStreamMethod("Upload", s.Upload).
		BidiStreamMethod("Chat", s.Chat)
}

// 模块装配
root := module.New(module.Metadata{
	Imports:     []module.Module{grpcadapter.Module(grpcadapter.Config{Addr: ":50051"})},
	Controllers: []any{svc},
})
```

## 2. 四种模式

| 模式 | 方法 | handler 形态 |
|------|------|--------------|
| unary | `Method` | 单请求 → 单响应 |
| server stream | `ServerStreamMethod` | 首请求 → 多次 `Stream.Send` |
| client stream | `ClientStreamMethod` | 多次 `Stream.Recv` → 单响应 |
| bidi | `BidiStreamMethod` | 循环 `Recv` / `Send` |

## 3. CompositeStruct 写法

```go
type pingArgs struct {
	Ctx stdctx.Context
	Req *emptypb.Empty `req:"true" required:"true"`
}

type watchArgs struct {
	Req    *emptypb.Empty                         `req:"true" required:"true"`
	Stream grpcbinding.ServerStream[*emptypb.Empty]
}

func (s *UserService) Watch(args watchArgs) error {
	return args.Stream.Send(&emptypb.Empty{})
}
```

## 4. 请求消息推断

编译期会按以下顺序推断请求类型：

1. `MethodDefinition.RequestType`（显式钉死）
2. 参数中的 `Req[T]` wrapper
3. 参数中的 proto message
4. CompositeStruct 里带 `req:"true"` 的字段
5. **都没有时默认 `google.protobuf.Empty`**

因此下面这种"只依赖上下文"的方法可以正常启动：

```go
func (s *UserService) Me(hc *runtime.HandlerContext) (any, error) {
	...
}
```

## 5. 响应编码

handler 可以直接返回：

- `proto.Message`（原样返回）
- `map[string]any` / Go struct / slice / 标量（自动转 `Struct` / `ListValue` / `Value`）

```go
func (s *UserService) List(args listUsersArgs) (any, error) {
	return map[string]any{
		"list":  items,
		"total": total,
		"user":  userView{ID: 1, Name: "Tom"}, // Go struct 也可以
	}, nil
}
```

需要手动构造 `*structpb.Struct` 时：

```go
s, err := grpcvalue.ToStruct(map[string]any{"account": accountView{...}})
```

{{< callout type="warning" >}}
`structpb.NewStruct` 只接受 JSON 基础类型，嵌套 Go struct 会 panic。
请使用 `grpcvalue.ToStruct`。
{{< /callout >}}

## 6. 元数据与鉴权

```go
func authGuard(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
	hc := ctx.HandlerContext.(*runtime.HandlerContext)
	token := hc.Request.Headers["authorization"]
	// 校验 token
	return next.Handle()
}
```

客户端：

```go
ctx := metadata.AppendToOutgoingContext(ctx,
	"authorization", "Bearer "+token,
	"x-trace-id", "trace-001",
)
```

## 7. 调试

示例开启了 gRPC reflection：

```bash
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext -d '{"username":"demo","password":"123456"}' \
  localhost:50051 AuthService/Login
```

没有 `grpcurl` 时，可以用 Go 动态客户端（见 `cmd` 中的 E2E 实践）：
`grpc.NewClient` + `conn.Invoke("/AuthService/Register", req, resp)`，
消息体使用 `structpb.Struct`。
