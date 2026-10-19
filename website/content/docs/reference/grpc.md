---
title: gRPC
weight: 7
prev: /docs/reference/http
next: /docs/reference/graphql
---

包：`github.com/sao-lang/lania-g/protocol/grpc/v3`

## 1. 模块入口

```go
func Module(cfg Config) module.Module
```

| Config 字段 | 说明 |
|-------------|------|
| `Addr` | 监听地址，例如 `":50051"` |
| `Server` | 可注入预配置的 `*grpc.Server`（如启用 reflection） |

```go
server := grpc.NewServer()
reflection.Register(server)

root := module.New(module.Metadata{
	Imports:     []module.Module{grpcadapter.Module(grpcadapter.Config{Addr: ":50051", Server: server})},
	Controllers: []any{&UserService{}},
})
```

> 想在本地用 `grpcurl` 调试，务必 `reflection.Register(server)`。

## 2. Service 声明

```go
type Controller interface {
	Declare(*Routes)
}
```

```go
func (s *UserService) Declare(r *grpc.Routes) {
	r.Service("UserService").
		Method("Get", s.Get).
		Method("Create", s.Create).WithReqType((*structpb.Struct)(nil)).
		ServerStreamMethod("Watch", s.Watch)
}
```

| 类型 | 方法 |
|------|------|
| `Routes` | `Service(name) *ServiceBuilder` |
| `ServiceBuilder` | `Method` / `ServerStreamMethod` / `ClientStreamMethod` / `BidiStreamMethod` |
| | `UseMiddlewares` / `UseGuards` / `UseInterceptors` / `UsePipes` / `UseFilters` / `UseParamPipes` |
| | `Err()`（声明结束时会自动 seal） |
| `MethodBuilder` | `WithReqType(msg)` / `WithRespType(msg)` / `BindParam(index, name)` / 各类 AOP |

服务名与 RPC 全路径 = `/<service>/<method>`。

## 3. 四种调用模式

### Unary

```go
func (s *UserService) Get(req *pb.GetRequest) (*pb.UserReply, error)
```

不声明请求字段时，框架自动用 `google.protobuf.Empty` 作为请求类型。

### ServerStream

```go
func (s *StreamService) WatchUsers(req *pb.WatchRequest, stream grpcsrv.ServerStream[pb.UserReply]) error {
	for _, u := range s.store.list() {
		if err := stream.Send(u); err != nil { return err }
	}
	return nil
}
```

### ClientStream

```go
func (s *StreamService) UploadUsers(stream grpcsrv.ClientStream[pb.UserReply]) error {
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) { break }
		if err != nil { return err }
		// ...
	}
	return nil
}
```

### BidiStream

```go
func (s *StreamService) ChatUsers(stream grpcsrv.BidiStream[pb.UserReply, pb.UserReply]) error {
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) { return nil }
		if err != nil { return err }
		_ = stream.Send(msg)
	}
}
```

## 4. 请求类型推断

| 场景 | 请求类型 |
|------|----------|
| 首参实现 `proto.Message` | 该消息 |
| 首参为 `binding.Req[T]` | `T` |
| 首参为普通 struct | 由 `structpb.Struct` 承载，再用 `grpcvalue.ToStruct` 转换 |
| 没有请求字段 | `google.protobuf.Empty` |

```go
func (s *UserService) Create(req structpb.Struct) (*userView, error) {
	var body createReq
	if err := grpcvalue.ToStruct(req, &body); err != nil { return nil, err }
	...
}
```

## 5. 响应桥接 `grpcvalue`

gRPC 要求返回 `proto.Message`，但业务常常直接返回 struct。

| 函数 | 用途 |
|------|------|
| `grpcvalue.ToMessage(v any) (proto.Message, error)` | 任意值 → proto.Message |
| `grpcvalue.ToStruct(v any, dst any) error` | `structpb.Struct` → 目标 struct |

```go
// 直接返回 struct，框架自动 ToMessage
func (s *UserService) List() ([]userView, error) { ... }
```

> **坑**：`structpb.NewStruct` 只接受 `map[string]any`，传 Go struct 会 panic，必须走 `grpcvalue.ToStruct`。

## 6. metadata 与鉴权

```go
type listArgs struct {
	Meta  grpcbinding.Headers              // 全部 metadata
	Trace grpcbinding.Header[string]       `metadata:"x-trace-id"`
}

app.UseGlobalInterceptors(func(ctx *aop.InvocationContext, next func() (any, error)) (any, error) {
	md, _ := metadata.FromIncomingContext(ctx.Context())
	tokens := md.Get("authorization")
	...
	ctx.Set(authAccountKey, acc)
	return next()
})
```

## 7. 错误传递

gRPC adapter **不会**自动把 `KernelError.Kind` 映射成 gRPC status code：handler 返回的 error 会原样抛给 gRPC server，非 `status.Status` 的错误最终表现为 `codes.Unknown`。

```go
// 推荐：业务直接返回带 code 的错误
if user == nil {
	return nil, status.Error(codes.NotFound, "user not found")
}
```

若希望统一映射，用 server interceptor 一次搞定：

```go
func grpcErrorInterceptor(
	ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
) (any, error) {
	resp, err := handler(ctx, req)
	if err == nil {
		return resp, nil
	}
	var kerr *kerrors.KernelError
	if errors.As(err, &kerr) {
		switch kerr.Kind {
		case kerrors.KindValidation:
			return nil, status.Error(codes.InvalidArgument, kerr.Error())
		case kerrors.KindUnauthorized:
			return nil, status.Error(codes.Unauthenticated, kerr.Error())
		case kerrors.KindForbidden:
			return nil, status.Error(codes.PermissionDenied, kerr.Error())
		case kerrors.KindRouteNotFound:
			return nil, status.Error(codes.NotFound, kerr.Error())
		}
	}
	var httpErr *aop.HttpException
	if errors.As(err, &httpErr) {
		return nil, status.Error(grpcCodeFromHTTP(httpErr.Status), httpErr.Error())
	}
	return nil, status.Error(codes.Internal, err.Error())
}

grpcAdp.WithServerOptions(grpc.UnaryInterceptor(grpcErrorInterceptor))
```

## 8. 与 AOP 的组合顺序

```text
middleware → guard → interceptor → pipe → handler
                                    ↓
                             exception filter
```

`app.UseGlobalInterceptors` 会作用于所有适配器，注意 HTTP 与 gRPC 的拦截器语义不同（gRPC 里 `ctx.Set` 的扩展数据通过 `binding.GRPCContext` 读取）。
