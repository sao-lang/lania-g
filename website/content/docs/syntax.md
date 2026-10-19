---
title: 语法速查
weight: 3
toc: false
---

一页看全所有 DSL。左侧是签名，右侧是形态说明。详细行为见 [参考手册](reference/)。

## 1. 模块与 DI

```go
// 模块（全仓只有两个构造入口）
module.New(module.Metadata{Imports, Providers, Controllers, Resolvers, Exports, AOP})
module.AsGlobal(module.Metadata{...})
module.ForwardRef(func() module.Module { ... })   // 打破循环引用

// Provider
di.ProviderFromInstance(instance)
di.ProviderFromInstanceWithToken(reflect.TypeFor[T](), instance, di.Singleton)
di.ProviderFromFactory(token, func() (T, error) { ... }, di.Singleton)

// 作用域
di.Singleton / di.Transient / di.Request

// 取值
container.Get(reflect.TypeFor[T]())
di.GetByType[T](container)
module.GetByType[T](modRef)
```

## 2. Application

```go
app, err := application.New(root, application.Options{Logger: os.Stdout})
app.UseGlobalMiddlewares(...)
app.UseGlobalGuards(...)
app.UseGlobalInterceptors(...)
app.UseGlobalPipes(...)
app.UseGlobalFilters(...)
app.Run()      // 声明 → 编译 → 启动 → 阻塞
app.Stop()
app.Diagnostics()
app.HotLoad(module.Module)
```

## 3. AOP 五类增强

```go
// Middleware
aop.MiddlewareFunc(func(ctx *aop.ExecutionContext, next func() error) error { return next() })

// Guard：返回是否放行
aop.GuardFunc(func(ctx *aop.ExecutionContext) (bool, error) { return true, nil })

// Interceptor：包裹 handler 调用
aop.InterceptorFunc(func(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
	return next.Handle()
})

// Pipe / ExceptionFilter
aop.PipeFunc(func(value any, metadata any) (any, error) { return value, nil })
aop.ExceptionFilterFunc(func(err any, ctx *aop.ExecutionContext) error { return nil })
```

## 4. HTTP

```go
http.Module(http.Config{Addr: ":8080", Validator: true})

func (c *X) Declare(r *http.Routes) {
	r.Prefix("/users").
		UseGuards(...).
		Get("/:id", c.Get).Summary("...").Tags("users").Security("bearerAuth").
		Post("", c.Create).Status(201)
}
```

## 5. gRPC

```go
grpc.Module(grpc.Config{Addr: ":50051", Server: srv})

func (s *X) Declare(r *grpc.Routes) {
	r.Service("UserService").
		Method("Get", s.Get).
		ServerStreamMethod("Watch", s.Watch)
}
```

## 6. GraphQL

```go
graphql.Module(graphql.Config{Addr: ":8082"})   // resolver 放 Resolvers 槽位

func (q *X) Declare(r *graphql.Routes) {
	r.Resolver("Query").Query("user", q.User).Arg("id").Returns("UserResult")
}
```

## 7. WebSocket

```go
ws.Module(ws.Config{Addr: ":8081"})

func (g *X) Declare(r *ws.Routes) {
	r.Gateway("/ws/chat").On("ping", g.Ping)
}
```

## 8. MQ

```go
mq.Module(mq.Config{Driver: driver})

func (c *X) Declare(r *mq.Routes) {
	r.Consumer("default").Group("g1").On("topic", c.Handle).BindParam(1, "X-Trace-Id")
}
```

## 9. Scheduler

```go
scheduler.Module(scheduler.Config{})

func (w *X) Declare(r *scheduler.Routes) {
	r.Job("cleanup").Cron("0 3 * * *", w.Cleanup).Retry(3, time.Second)
}
```

## 10. Binding wrapper 一览

| 协议 | wrapper |
|------|---------|
| HTTP | `Context` `Param[T]` `Query[T]` `Header[T]` `Headers` `Body[T]` `BodyAs[T]` `BodyBytes` `Form[T]` `Cookie[T]` `Cookies` `File` `Files` `Bind[T]` `MustBind[T]` `Original` `Next` `IP/Host/Method/Path/URL` `UploadedFile` `Auth*` |
| gRPC | `Req[T]` `Header[T]` `Headers` `Metadata` `Context` `FullMethod/Service/Method` `RawServerStream` `ServerStream[T]` `ClientStream[T]` `BidiStream[Req,Resp]` `GRPCContext` |
| GraphQL | `Arg[T]` `ArgValue[T]` `Parent[T]` `Header[T]` `Headers` `Context` `Variables` `Extensions` `SelectionSet` `Root` `Info` `OperationName` `FieldName` `RawQuery` `IP/Host/Method/URL/Path/Session` `Request` `Response` |
| WS | `WSMessageBody[T]` `Context` `Header[T]` `WSHeaders[T]` `Headers` `WSConnectedSocket` `WSEvent` `WSSocketID` `WSRooms` |
| MQ | `Message[T]` `Header[T]` `Headers` `Topic` `Consumer` `Key` `RetryCount` `Ack` `Nack` `Context` |
| Scheduler | `JobName` `TriggerType` `RunID` `ScheduledAt` `Context` |

参数标签：

```go
httpbinding.Param[int]     `param:"id" required:"true"`
httpbinding.Query[string]  `query:"keyword"`
httpbinding.Header[string] `header:"X-Trace-Id"`
grpcbinding.Header[string] `metadata:"x-trace-id"`
gqlbinding.Arg[int]        `arg:"id" required:"true"`
gqlbinding.Header[string]  `header:"x-trace-id"`
validate:"required,email,min=1"   // validator v10
```

## 11. 集成装配

```go
import (
	httpclient "github.com/sao-lang/lania-g/integrations/http/v3"
	grpcclient "github.com/sao-lang/lania-g/integrations/grpc/v3"
)

logger.Module(logger.Config{Level: logger.InfoLevel})
cache.Module(cache.Config{Type: cache.Memory})
config.Module(config.Config{Files: []string{"config.yaml"}, EnvPrefix: "APP"})
auth.Module(auth.Config{JWTSecret: "change-me", JWTIssuer: "lania"})
orm.Module(orm.Config{Dialector: sqlite.Open(dsn), AutoMigrate: []any{&User{}}})
mongodb.Module(mongodb.Config{URI: "mongodb://...", Database: "app"})
kafka.Module(kafka.Config{Brokers: []string{"127.0.0.1:9092"}})
minio.Module(minio.Config{Endpoint: "127.0.0.1:9000", AccessKeyID: "minio", SecretAccessKey: "minio123"})
es.Module(es.Config{Addresses: []string{"http://127.0.0.1:9200"}})
otel.Module(otel.Config{ServiceName: "app"})
outbox.Module(outbox.Config{Store: store, Dispatcher: d, MaxAttempts: 5})
resilience.Module(resilience.Config{RateLimit: ..., Retry: ..., Circuit: ...})
events.Module(events.Config{Async: true})
httpclient.Module(httpclient.Config{BaseURL: "https://api.example.com", Timeout: time.Second})
grpcclient.Module(grpcclient.Config{Target: "127.0.0.1:50051", Insecure: true})
swagger.Module(swagger.Config{Title: "API", Version: "1.0.0"}, &swagger.UIConfig{...})
terminus.Module(terminus.Config{Version: "1.0.0"})
```

## 12. 常用命令

```bash
go build ./...            # 单模块构建（每个 go.mod 目录各跑一次）
go test ./...
go vet ./...
hugo --source website     # 本地文档站
```
