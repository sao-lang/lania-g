---
title: 语法速查
weight: 3
toc: false
---

一页看全所有 DSL。左侧是签名，右侧是形态说明。详细行为见 [参考手册](reference/)。

## 1. 模块与 DI

```go
// 模块
module.CreateModule(imports, providers, controllers, resolvers, exports)
module.CreateGlobalModule(imports, providers, controllers, resolvers, exports)
module.CreateAutoModule(imports, exports)
module.CreateAsyncModule(imports, factory)
module.NewBaseModule(&module.ModuleMetadata{Imports: ..., Controllers: ...})
module.NewGlobalModule(&module.ModuleMetadata{...})
module.ModuleDecorator(&module.ModuleMetadata{...})   // 函数式装饰
module.ForwardRef(func() module.Module { ... })       // 打破循环引用
module.ForwardRefType(reflect.TypeFor[T]())

// Provider
di.ProviderFromInstance(instance)
di.ProviderFromInstanceWithToken(reflect.TypeFor[T](), instance, di.Singleton)
di.ProviderFromFactory(func(c *di.Container) (T, error) { ... })
di.ProviderFromType(reflect.TypeFor[T](), factory)
di.ProviderFromTypeWithToken(token, reflect.TypeFor[T](), factory, di.Transient)

// 作用域
di.Singleton   // 容器内唯一
di.Transient   // 每次新建
di.Request     // 请求级（HTTP/gRPC 各自上下文）

// 取值
container.Get(reflect.TypeFor[T]())
di.GetByType[T](container)
module.GetByType[T](modRef)      // 在模块树中按类型查找导出
di.Injectable                    // 标记结构体可被字段注入
```

## 2. Application

```go
app, err := application.New(root, application.Options{
	Registry: application.NewRegistry(),   // 必填
	StartupReporter: os.Stdout,            // 可选
}, adapters...)
application.New(root, adapters...)
application.NewCompat(root, adapters...)

app.UseGlobalMiddlewares(aop.MiddlewareFunc(...))
app.UseGlobalGuards(aop.GuardFunc(...))
app.UseGlobalInterceptors(interceptor)
app.UseGlobalPipes(aop.PipeFunc(...))
app.UseGlobalFilters(aop.ExceptionFilterFunc(...))

app.CompileDiagnostics()
app.StartupReport()
app.GraphDiagnostics()
app.HotLoad(module.Module)
app.Run()                // 阻塞
app.Run()      // 阻塞
app.Stop()
```

## 3. AOP 五类增强

```go
// Middleware：包裹整个执行过程
func(ctx *aop.ExecutionContext, next aop.MiddlewareHandler) (any, error)

// Guard：返回是否放行
aop.GuardFunc(func(ctx *aop.ExecutionContext) (bool, error) { ... })

// Interceptor：包裹 handler 调用
aop.InterceptorFunc(func(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
	return next.Handle()
})

// Pipe：入参/返回值变换
aop.PipeFunc(func(value any, metadata any) (any, error) { ... })

// ExceptionFilter：消费异常
aop.ExceptionFilterFunc(func(exception any, ctx *aop.ExecutionContext) error { ... })

// 异常
aop.BadRequestException(msg) / UnauthorizedException / ForbiddenException
aop.NotFoundException / ConflictException / InternalServerErrorException
```

## 4. HTTP

```go
adp := httpadapter.New()               // 共享监听
adp := httpadapter.New(":8080")        // 独立监听
adp.WithBasePath("/api").WithMaxBodyBytes(1<<20).
	WithValidatorV10().EnableCors(nil).EnableHelmet(nil).
	ServeStatic("/assets", "./public").
	MountHandler("/metrics", h).
	WithNextHandler(fallback).
	UseAfter(mw...)

api := adp.API().(*httpadapter.API)
api.Controller("/users", ctrl).
	UseMiddlewares(...).UseGuards(...).UseInterceptors(...).
	UsePipes(...).UseFilters(...).UseParamPipes(0, pipe).
	Get("/", ctrl.List).Summary("列表").Tags("users").
	Post("/", ctrl.Create).
	Get("/:id", ctrl.Get).ResponseBody(userView{}).
	Put("/:id", ctrl.Update).
	Patch("/:id", ctrl.Patch).
	Delete("/:id", ctrl.Delete).
	Head("/:id", ctrl.Head).
	Options("/:id", ctrl.Options).
	All("/any", ctrl.Any).
	Get("/old", ctrl.Old).Redirect("/new", 301).
	Get("/page", ctrl.Page).Render("page.html").
	Get("/hidden", ctrl.H).HideFromDocs().
	BuildE()

api.MountHandler("/debug", debugHandler)
```

## 5. gRPC

```go
adp := grpcadapter.New(":50051").WithServer(grpc.NewServer()).
	WithServerOptions(grpc.UnaryInterceptor(i))
api := adp.API().(*grpcadapter.API)

api.Service("UserService", svc).
	UseGuards(...).UseInterceptors(...).UsePipes(...).UseFilters(...).
	Method("Get", svc.Get).
	Method("Create", svc.Create).WithReqType((*structpb.Struct)(nil)).
	ServerStreamMethod("Watch", svc.Watch).
	ClientStreamMethod("Upload", svc.Upload).
	BidiStreamMethod("Chat", svc.Chat).
	BuildE()
```

## 6. GraphQL

```go
adp := graphqladapter.New().WithPlayground(true).WithPath("/graphql").
	WithValidatorV10().WithErrorFormatter(fn).WithMaxBodyBytes(1<<20)
api := adp.API().(*graphqladapter.API)

api.UseComplexityLimit(200)
api.DisableIntrospection()
api.UseExtensions(ext)
api.UseSchema(schema)

api.Resolver("Query", r).
	Query("users", r.Users).Arg("page").Arg("size").Returns("UserListResult").
	Object("detail", r.Detail).Mutation("create", r.Create).
	Description("...").Deprecation("...").CacheControl("private").
	Complexity(5).RateLimit(20, 1000).Timeout(5000).
	RequirePermission("user:create").
	UseGuards(...).UseInterceptors(...).UsePipes(...).UseFilters(...).
	BuildE()

// ArgOption
graphqladapter.WithRequired(true) / WithDescription("...") / WithDefault(v)
```

## 7. WebSocket

```go
adp := wsadapter.New().WithSocketPath("/socket.io/").
	WithResponseEventSuffix("Response").WithErrorEventSuffix("Error")
api := adp.API().(*wsadapter.API)

api.Gateway("/ws/chat", gw).
	UseMiddlewares(...).UseGuards(...).UsePipes(...).UseFilters(...).
	On("event", gw.Handle).
		UseGuards(...).UseInterceptors(...).
	On("other", gw.Other).
	BuildE()

adp.BroadcastToRoom("/ws/chat", "r1", "msg", payload)
adp.BroadcastToNamespace("/ws/chat", "msg", payload)
adp.RoomLen("/ws/chat", "r1")
```

## 8. MQ

```go
adp := mqadapter.New(mqkafka.New(mqkafka.Config{Brokers: []string{...}}))
api := adp.API().(*mqadapter.API)

api.Consumer("order", consumer).
	Group("order-group").Concurrency(4).
	On("order.created", consumer.OnCreated).BindParam(2, "X-Trace-Id").
	On("order.paid", consumer.OnPaid).
	BuildE()
```

## 9. Scheduler

```go
adp := scheduleradapter.New()
api := adp.API().(*scheduleradapter.API)

api.Job("name", worker).
	Cron("0 3 * * *", worker.Run).   // 或 Every(d, fn) / After(d, fn)
	Retry(3, 5*time.Second).
	MaxConcurrency(1).
	Unique("name").
	WithTimeout(30*time.Second).
	Misfire(scheduleradapter.MisfireSkip).   // 或 MisfireQueue
	BindParam(1, "job").
	BuildE()

adp.SetUniqueLocker(locker)
adp.SetHistoryLimit(50)
scheduleradapter.MountHTTPBridge(app, adp, "/scheduler/status")
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
