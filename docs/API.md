# lania-g v3 API 文档

## 1. 文档范围

本文档描述当前多 module 版本的公开使用面，重点覆盖以下内容：

- 应用与模块的标准装配方式
- 默认实例、命名实例与 `Factory` 的统一模型
- `Application`、`Module`、`DI` 的公开 API
- 各协议适配器的 DSL 与启动方式
- 多协议自动绑定类型与常见参数
- 请求期四级增强与生命周期接口
- `integrations/*` 的标准接入模式与常见案例
- 公共包与内部基础设施包的边界

本文档以当前代码实现为准，适用于业务应用接入、框架扩展与 API 使用约束说明。

## 2. 标准使用模型

### 2.1 基本链路

`lania-g v3` 的标准接入路径如下：

```text
Module
  -> Application
  -> Adapter 挂载
  -> Compile / Install
  -> Listen / Run
```

各阶段职责如下：

- `Module`
  - 定义 `imports`、`providers`、`controllers`、`resolvers`、`exports`
  - 承载实例组织与依赖注入
- `Application`
  - 负责装配模块、运行时、注册中心、适配器与协议插件
  - 负责触发编译、安装与生命周期执行
- `Adapter`
  - 负责协议 DSL、transport 与协议插件
- `Listen / Run`
  - 负责共享监听或独立监听模式下的应用启动

### 2.2 标准示例

```go
package main

import (
	adapterhttp "github.com/sao-lang/lania-g/protocol/http/v3"
	"github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

func buildRootModule() module.Module {
	return module.CreateModule(nil, nil, nil, nil, nil)
}

func main() {
	root := buildRootModule()
	httpAdapter := adapterhttp.New()

	app, err := application.NewWithOptions(
		root,
		application.Options{
			Registry: application.NewRegistry(),
		},
		httpAdapter,
	)
	if err != nil {
		panic(err)
	}

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
```

### 2.3 启动模式

当前框架支持两类 adapter 启动方式：

- 共享监听模式
  - 通过 `Application.Listen(addr)` 统一绑定端口
  - 常见于 HTTP、GraphQL、WS
- 独立监听模式
  - 由 adapter 自身维护监听地址
  - 通过 `Application.Run()` 启动

若应用中同时存在共享监听与独立监听 adapter，推荐使用：

```go
app.Listen(":8080")
```

由 `Application` 统一协调。

## 3. 资源获取模型

当前框架对外统一采用以下资源获取方式：

- `ForRoot(...)` / `ForRoots(...)` / `ForRootMulti(...)`
  - 将实例注册到模块容器
  - 供 handler 自动绑定与容器取值使用
- `Factory`
  - 用于主动获取默认实例或命名实例
- 类型化取值 helper
  - `module.GetByType[T]` / `module.MustGetByType[T]`
  - `di.GetByType[T]` / `di.MustGetByType[T]`

### 3.1 默认实例

默认实例表示某类资源的主实例。

示例：

```go
func Handle(db orm.InjectDataSource, log logger.InjectLogger) error {
	_ = db.DB
	log.Info("ok")
	return nil
}
```

### 3.2 命名实例

命名实例表示同一类资源下的多个不同实例，通过名称区分。

示例：

```go
type AnalyticsDB struct{}

func (AnalyticsDB) ORMDataSourceName() string { return "analytics" }

func Handle(db orm.DataSourceRef[AnalyticsDB]) error {
	_ = db.DB
	return nil
}
```

### 3.3 Factory

`Factory` 用于在业务或扩展代码中主动获取实例。

```go
ormFactory := module.MustGetByType[orm.Factory](app.ModuleRef())
defaultDB := ormFactory.Default()
analyticsDB, err := ormFactory.GetOrCreate("analytics", cfg)
```

## 4. Application API

包：`github.com/sao-lang/lania-g/application/v3`

### 4.1 创建应用

```go
app, err := application.NewWithOptions(rootModule, application.Options{
	Registry:        application.NewRegistry(),
	StartupReporter: os.Stdout, // 可选：启动时输出 startup report
}, adapters...)
```

历史便捷入口仍然保留：

```go
app, err := application.New(rootModule, adapters...)
```

`New(...)` 仍可用，但它会沿用历史上的默认回退语义，因此不作为业务模板的默认入口。

如果你正在维护旧的全局 registry 路径，可显式使用迁移层 compat 入口：

```go
app, err := application.NewCompat(rootModule, adapters...)
```

### 4.2 稳定公开能力

`Application` 当前提供以下稳定能力：

- `Runtime()`
- `Registry()`
- `ModuleRef()`
- `CompileDiagnostics()`
- `LastCompileDiagnostics()`
- `StartupReport()`
- `UseAdapter(...)`
- `RegisterPlugin(...)`
- `HotLoad(module)`
- `Run()`
- `Listen(addr)`
- `Stop()`
- `UseGlobalMiddlewares(...)`
- `UseGlobalGuards(...)`
- `UseGlobalInterceptors(...)`
- `UseGlobalPipes(...)`
- `UseGlobalFilters(...)`
- `SetGlobalPrefix(prefix)`

其中入口建议分层如下：

- 推荐入口：`NewWithOptions(...)`
- 便捷入口：`New(...)`
- 显式兼容入口：`NewCompat(...)`

### 4.3 编译与启动诊断

`Application` 当前提供两类应用级诊断能力：

- `CompileDiagnostics()`
  - 在真正启动前触发编译，并返回协议、声明、路由、冲突、binding resolver 与全局 AOP 汇总
- `LastCompileDiagnostics()`
  - 返回最近一次编译诊断快照
- `StartupReport()`
  - 返回启动视角下的汇总信息，包括已加载模块、已挂载 adapter、协议数量、路由数量、全局 AOP 与 integrations 摘要
  - 若探测到全局 compat 写路径，固定分段 `compatFallbacks:` 会先输出 `fallbackCategories=...` 的分类摘要，再保留 `fallbackSources=...` 的来源明细
  - 结构化读取时，可直接使用 `CompileDiagnostics` / `StartupReport` 上的 compat fallback summary 字段，而不必解析字符串
  - JSON 输出键名已固定为 `lowerCamelCase`，避免外部消费方绑定 Go 字段名

JSON 稳定输出约定：

- `CompileDiagnostics` 可稳定依赖的顶层键：
  - `registeredPlugins`
  - `protocolOrder`
  - `declarationCounts`
  - `bindingResolverCount`
  - `registrySource`
  - `globalAOP`
  - `protocols`
  - `registryFallbacks`
  - `runtimeFallbacks`
  - `compatFallbackCategories`
  - `compatFallbackSources`
  - `routeConflicts`
  - `errors`
  - `warnings`
- `StartupReport` 可稳定依赖的顶层键：
  - `moduleCount`
  - `adapterCount`
  - `protocolCount`
  - `routeCount`
  - `bindingResolverCount`
  - `registrySource`
  - `compatFallbackCategories`
  - `compatFallbackSources`
  - `globalAOP`
  - `registryFallbacks`
  - `runtimeFallbacks`
  - `modules`
  - `adapters`
  - `protocols`
  - `integrations`
  - `warnings`
- compat 聚合子项的稳定键：
  - `compatFallbackCategories[*]` 使用 `category` / `hits` / `sources`
  - `compatFallbackSources[*]` 使用 `source` / `hits`
- 若未来需要新增键，会优先采用“只增不删、不改已有键名”的兼容策略。

示例：

```go
diag, err := app.CompileDiagnostics()
if err != nil {
	panic(err)
}
report, err := app.StartupReport()
if err != nil {
	panic(err)
}
// 结构化读取 compat 聚合结果
_ = diag.CompatFallbackCategories
_ = diag.CompatFallbackSources
_ = report.CompatFallbackCategories
_ = report.CompatFallbackSources
_ = diag
_ = report
```

示例 JSON 片段：

```json
{
  "bindingResolverCount": 6,
  "registrySource": "instance",
  "compatFallbackCategories": [
    {
      "category": "eventsCompatWrites",
      "hits": 1,
      "sources": 1
    }
  ],
  "compatFallbackSources": [
    {
      "source": "integrations/events.RegisterOnCompat",
      "hits": 1
    }
  ],
  "warnings": [
    "ignored migration-only global events compat writes detected (...)"
  ]
}
```

```json
{
  "moduleCount": 3,
  "adapterCount": 1,
  "protocolCount": 1,
  "routeCount": 4,
  "registrySource": "instance",
  "compatFallbackCategories": [
    {
      "category": "eventsCompatWrites",
      "hits": 1,
      "sources": 1
    }
  ],
  "protocols": [
    {
      "protocol": "http",
      "pluginId": "http",
      "declarations": 4,
      "routes": 4,
      "routeContainers": 1,
      "ownerModules": [
        {
          "moduleKey": "*example.UserModule",
          "routes": 4,
          "routeKeys": ["GET /users", "POST /users"]
        }
      ]
    }
  ]
}
```

若在 `application.Options` 中配置 `StartupReporter`，则 `app.Run()` / `app.Listen(addr)` 会在实际启动 adapter 前自动输出 startup report。

### 4.4 Registry 隔离

`Application.NewWithOptions(...)` 支持注入独立 `Registry`。该能力用于：

- 避免多个应用实例共享同一个声明池
- 降低测试之间的状态污染
- 使模块初始化、binding 注册与生命周期桥接绑定到当前应用实例

### 4.5 HotLoad

`Application.HotLoad(module)` 用于在应用实例创建后动态装载一个新模块，并立即重新编译、重建 `runtime`。

- 适用场景
  - 运行中追加新的 controller、resolver、provider 或 integration 模块
  - 在不重建 `Application` 实例的前提下刷新模块图
- 执行语义
  - 调用模块加载器将新模块加入当前应用
  - 重新执行编译并用新的 `runtime` 原子替换旧运行时
  - 若应用已经完成 `bootstrap`，则会对新模块补跑 `OnApplicationBootstrap`
  - 若某个 adapter 实现了 `Reload() error`，会在运行时切换后收到一次刷新通知
- 当前约束
  - `HotLoad` 以模块类型判重，同一类型的模块不能重复装载
  - 不同协议 adapter 的运行中刷新能力取决于其自身是否支持 `Reload()`

## 5. Module 与 DI API

包：

- `github.com/sao-lang/lania-g/kernel/v3/module`
- `github.com/sao-lang/lania-g/kernel/v3/di`

### 5.1 创建模块

```go
root := module.CreateModule(imports, providers, controllers, resolvers, exports)
```

### 5.2 ModuleRef 类型化取值

```go
builder := module.MustGetByType[*swagger.Builder](app.ModuleRef())
factory := module.MustGetByType[orm.Factory](app.ModuleRef())
```

### 5.3 Container 类型化取值

```go
loader := di.MustGetByType[*config.Loader](container)
client := di.MustGetByType[*http.Client](container)
```

### 5.4 Must 语义

- `MustGetByType(...)` 与 `MustGet(...)` 属于显式 Must API，失败时允许 panic
- 非 Must API 应优先使用 `GetByType(...)` / `Get(...)` 并处理错误

## 6. 请求期四级增强与生命周期

### 6.1 请求期四级增强

推荐按以下四级理解请求执行链中的增强点：

1. `Middleware`
   - 处理请求进入执行链之前的横切逻辑
   - 典型场景：访问日志、trace 注入、上下文补充
2. `Guard`
   - 决定请求是否允许继续执行
   - 典型场景：鉴权、权限校验、租户校验
3. `Interceptor`
   - 包裹 handler 执行前后过程
   - 典型场景：耗时统计、结果包装、事务边界
4. `Pipe`
   - 对参数进行转换、规范化与校验前处理
   - 典型场景：字段归一化、结构转换、默认值填充

此外：

- `Filter` 用于异常与错误转换，属于错误处理层

### 6.2 全局挂载

```go
// 以下名称仅表示典型增强点，需由应用自行实现。
app.UseGlobalMiddlewares(accessLogMiddleware)
app.UseGlobalGuards(authGuard)
app.UseGlobalInterceptors(traceInterceptor)
app.UseGlobalPipes(validationPipe)
app.UseGlobalFilters(exceptionFilter)
```

### 6.3 协议声明层挂载

HTTP 示例：

```go
httpAPI := httpAdapter.API().(*http.API)

httpAPI.Controller("/users", controller).
	UseGuards(authGuard).
	UseInterceptors(traceInterceptor).
	Get("/:id", controller.GetUser).
	Build()
```

gRPC 示例：

```go
grpcAPI := grpcAdapter.API().(*grpc.API)

grpcAPI.Service("UserService", svc).
	UseGuards(authGuard).
	UseInterceptors(metricsInterceptor).
	Method("GetUser", svc.GetUser).
	Build()
```

### 6.4 生命周期接口

当前生命周期接口主要包括：

- `OnModuleInit`
- `OnModuleDestroy`
- `OnApplicationBootstrap`
- `OnApplicationShutdown`

典型用途如下：

- `OnModuleInit`
  - 模块初始化完成后执行
  - 适合建立连接、初始化内部资源
- `OnModuleDestroy`
  - 模块销毁阶段执行
  - 适合关闭连接、释放资源
- `OnApplicationBootstrap`
  - 应用整体启动完成后执行
  - 适合桥接型逻辑，例如事件处理器自动挂载
- `OnApplicationShutdown`
  - 应用关闭前执行
  - 适合统一清理全局资源

## 7. Adapter API

本节默认只展开实例级 adapter API。包级 DSL 与各协议 `NewCompatAPI(...)` 归为迁移层入口，仅在维护历史全局声明代码时再回看。

### 7.1 HTTP

包：`github.com/sao-lang/lania-g/protocol/http/v3`

#### 创建 adapter

```go
httpadp := http.New()
```

常用配置：

- `WithBasePath(prefix)`
- `WithMaxBodyBytes(n)`
- `WithRenderer(r)`
- `WithValidator(v)`
- `WithValidatorV10()`
- `EnableCors(cfg)`
- `EnableHelmet(cfg)`
- `ServeStatic(prefix, root)`
- `MountHandler(pattern, handler)`

#### 路由 DSL

```go
httpAPI := httpadp.API().(*http.API)

httpAPI.Controller("/users", controller).
	UseGuards(authGuard).
	Get("/:id", controller.GetUser).
	Summary("Get User").
	Tags("users").
	Build()
```

#### DSL 错误处理

对于 controller-scope `Use*` 的误用，当前提供：

- `Build()`：兼容用法
- `BuildE() ([]*RouteDefinition, error)`：推荐用法
- `Err() error`：获取 builder 当前错误

### 7.2 gRPC

包：`github.com/sao-lang/lania-g/protocol/grpc/v3`

```go
grpcAPI := grpcAdapter.API().(*grpc.API)

grpcAPI.Service("UserService", svc).
	Method("GetUser", svc.GetUser).
	ServerStreamMethod("WatchUsers", svc.WatchUsers).
	ClientStreamMethod("UploadUsers", svc.UploadUsers).
	BidiStreamMethod("ChatUsers", svc.ChatUsers).
	Build()
```

对于 service-scope `Use*` 的误用，当前提供：

- `Build()`
- `BuildE() ([]*MethodDefinition, error)`
- `Err() error`

协议说明：

- `Method(...)`
  - 声明 unary RPC
- `ServerStreamMethod(...)`
  - 声明服务端流 RPC
- `ClientStreamMethod(...)`
  - 声明客户端流 RPC
- `BidiStreamMethod(...)`
  - 声明双向流 RPC

### 7.3 WebSocket

包：`github.com/sao-lang/lania-g/protocol/ws/v3`

```go
wsAdapter := ws.New().WithSocketPath("/socket.io/")
wsAPI := wsAdapter.API().(*ws.API)

wsAPI.Gateway("/chat", gateway).
	On("send_message", gateway.SendMessage).
	Build()
```

启动与挂载规则：

- `ws.New()`
  - 使用共享监听模式
  - 通过 `Application.Listen(addr)` 跟随 HTTP 一起挂载
- `ws.New(":3000")`
  - 使用独立监听模式
  - 通过 `Application.Run()` 启动 adapter 自己的 HTTP 服务
- `WithSocketPath("/socket.io/")`
  - 设置 socket.io 挂载路径
  - 默认路径是 `/socket.io/`
- `Gateway("/chat", gateway)`
  - 声明 namespace 与事件处理器
  - 事件通过 `On("event", handler)` 注册

### 7.4 GraphQL

包：`github.com/sao-lang/lania-g/protocol/graphql/v3`

```go
graphqlAPI := graphqlAdapter.API().(*graphql.API)

graphqlAPI.Resolver("User", resolver).
	Query("user", resolver.User).
	Build()
```

### 7.5 MQ

包：`github.com/sao-lang/lania-g/protocol/mq/v3`

```go
mqAPI := mqAdapter.API().(*mq.API)

mqAPI.Consumer("kafka-consumer", receiver).
	Group("user-group").
	On("user.created", receiver.HandleUserCreated).
	Build()
```

### 7.6 Scheduler

包：`github.com/sao-lang/lania-g/protocol/scheduler/v3`

```go
schedulerAPI := schedulerAdapter.API().(*scheduler.API)

schedulerAPI.Job("cleanup", service).
	Every(time.Minute, service.Run).
	Retry(3, time.Second).
	Unique("cleanup").
	Build()
```

附加能力：

- `MaxConcurrency(n)`
- `WithTimeout(d)`
- `Misfire(policy)`
- `Snapshot()`
- `MountHTTPBridge(...)`

## 8. 多协议自动绑定类型与常见参数

### 8.1 HTTP Binding

包：`github.com/sao-lang/lania-g/protocol/http/v3/binding`

常见绑定类型：

- 请求体
  - `Body[T]`
  - `BodyAs[T]`
  - `Bind[T]`
  - `MustBind[T]`
- 路径参数
  - `Param[T]`
- 查询参数
  - `Query[T]`
- 请求头
  - `Header[T]`
- 表单
  - `Form[T]`
- Cookie
  - `Cookie[T]`
  - `Cookies`
- 原始请求与标量
  - `Original`
  - `BodyBytes`
  - `MustBodyBytes`
  - `IP`
  - `Host`
  - `Method`
  - `Path`
  - `URL`
  - `Headers`
  - `Session`
- 文件上传
  - `File`
  - `Files`
- 认证相关
  - `AuthUser`
  - `AuthUserID`
  - `AuthToken`
  - `AuthOptionalUser`
  - `AuthOptionalToken`

示例：

```go
type CreateUserReq struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func (c *UserController) CreateUser(
	body bindinghttp.Body[CreateUserReq],
	trace bindinghttp.Header[string],
	ip bindinghttp.IP,
) error {
	_ = body.Value
	_ = trace.Value
	_ = ip
	return nil
}
```

### 8.2 gRPC Binding

包：`github.com/sao-lang/lania-g/protocol/grpc/v3/binding`

常见绑定类型：

- 请求体
  - `Req[T]`
  - 直接 proto message 参数
  - `CompositeStruct` + `req:"true"`
- 上下文
  - `Context`
  - `GRPCContext`
- 头部与 metadata
  - `Header[T]`
  - `Metadata`
  - `Headers`
  - `CompositeStruct` + `header:"..."`
- 上下文与方法信息
  - `HandlerContext`
  - `Context`
  - `FullMethod`
  - `Service`
  - `Method`
- 流能力
  - `RawServerStream`
  - `ServerStream[T]`
  - `ClientStream[T]`
  - `BidiStream[...]`
  - 上述类型既可作为顶层参数，也可放入 `CompositeStruct`

示例：

```go
type GetUserArgs struct {
	Req    *pb.GetUserRequest      `req:"true" required:"true"`
	Ctx    bindinggrpc.GRPCContext
	Meta   bindinggrpc.Metadata
	Method bindinggrpc.FullMethod
}

func (s *UserService) GetUser(args GetUserArgs) error {
	_ = args.Req
	_ = args.Ctx
	_ = args.Meta
	_ = args.Method
	return nil
}
```

说明：

- `bindinggrpc.Context`
  - 等价于标准库 `context.Context`
- `bindinggrpc.GRPCContext`
  - 是增强上下文
  - 提供 `HandlerContext()`、`Metadata()`、`FullMethod()`、`Service()`、`Method()`、`Header()`、`ShouldBindReq()`、`ShouldBindStream()`
- `ShouldBindReq(&dto)`
  - 仅用于 unary 与 server-stream 首请求
- `ShouldBindStream(msg, &dto)`
  - 仅用于 client-stream 与 bidi-stream 单条消息
- 当前 gRPC unary 主示例统一采用 `CompositeStruct`
- 兼容写法 `Req[T]`、直接 request message、`Header[T] + BindParam(...)` 仍可使用

### 8.3 WebSocket Binding

包：`github.com/sao-lang/lania-g/protocol/ws/v3/binding`

常见绑定类型：

- 消息体
  - `WSMessageBody[T]`
- 上下文
  - `Context`
- 头部
  - `Header[T]`
  - `WSHeaders[T]`
  - `Headers`
- 连接与事件信息
  - `WSConnectedSocket`
  - `WSEvent`
  - `WSSocketID`
  - `WSRooms`

示例：

```go
type ChatMessage struct {
	Room string `json:"room" validate:"required"`
	Text string `json:"text" validate:"required,min=1"`
}

func (g *ChatGateway) SendMessage(
	ctx bindingws.Context,
) error {
	var dto ChatMessage
	if err := ctx.ShouldBindMessage(&dto); err != nil {
		return err
	}
	_ = ctx.BroadcastTo(dto.Room, "message", map[string]any{"text": dto.Text})
	return nil
}
```

说明：

- `bindingws.Context`
  - 是 WS 增强上下文
  - 提供 `HandlerContext()`、`Event()`、`Namespace()`、`ID()`、`URL()`、`RemoteAddr()`、`Query()`、`Headers()`、`Request()`、`Message()`、`Conn()`、`Server()`、`Emit()`、`Join()`、`Leave()`、`Rooms()`、`BroadcastTo()`、`BroadcastToNamespace()`、`RoomLen()`、`LeaveAll()`、`Disconnect()` 与 `ShouldBindMessage()` 能力
- `ShouldBindMessage(&dto)`
  - 是当前推荐的消息 DTO 绑定入口
- `WSMessageBody[T]`
  - 仍然可用于显式声明消息体来源

### 8.4 GraphQL Binding

包：`github.com/sao-lang/lania-g/protocol/graphql/v3/binding`

常见绑定类型：

- 参数与父对象
  - `Arg[T]`
  - `ArgValue[T]`
  - `Parent[T]`
- 头部
  - `Header[T]`
  - `Headers`
- 查询上下文
  - `Context`
  - `Variables`
  - `Extensions`
  - `SelectionSet`
  - `Root`
  - `Info`
- 标量信息
  - `OperationName`
  - `FieldName`
  - `RawQuery`
  - `IP`
  - `Host`
  - `Method`
  - `URL`
  - `Path`
  - `Session`
- 原始 HTTP 对象
  - `Request`
  - `Response`

示例：

```go
type CreateUserArgs struct {
	Name string `json:"name" validate:"required"`
}

func (r *UserResolver) CreateUser(ctx bindinggraphql.Context) (any, error) {
	var args CreateUserArgs
	if err := ctx.ShouldBindArgs(&args); err != nil {
		return nil, err
	}
	return map[string]any{"name": args.Name}, nil
}
```

说明：

- `bindinggraphql.Context`
  - 当前 GraphQL 增强上下文
  - 提供 `Request()`、`Writer()`、`OperationName()`、`Query()`、`Variables()`、`Headers()`、`Extensions()`、`FieldType()`、`FieldName()`、`Path()`、`SelectionSet()`、`Root()`、`Info()`、`Args()`、`Session()`、`Header()`、`Var()`、`Arg()` 与 `ShouldBindArgs()`
- `ShouldBindArgs(&dto)`
  - 把当前字段的 args 绑定到 DTO
  - 在可用时使用 validator 执行结构体校验
- `Arg[T]` / `ArgValue[T]`
  - 仍然适合简单的单参数显式注入场景

### 8.5 MQ Binding

包：`github.com/sao-lang/lania-g/protocol/mq/v3/binding`

常见绑定类型：

- 消息体
  - `Message[T]`
- 头部
  - `Header[T]`
  - `Headers`
- 上下文
  - `Context`
- 元信息
  - `Topic`
  - `Consumer`
  - `Key`
  - `RetryCount`
- ack/nack
  - `Ack`
  - `Nack`

示例：

```go
type UserCreated struct {
	Id string `json:"id"`
}

func (c *Consumer) HandleUserCreated(
	ctx bindingmq.Context,
	msg bindingmq.Message[UserCreated],
	topic bindingmq.Topic,
	ack bindingmq.Ack,
) error {
	_ = ctx
	_ = msg.Value
	_ = topic
	return ack()
}
```

### 8.6 Scheduler Binding

包：`github.com/sao-lang/lania-g/protocol/scheduler/v3/binding`

常见绑定类型：

- 上下文
  - `Context`
- 调度元信息
  - `JobName`
  - `TriggerType`
  - `RunID`
  - `ScheduledAt`

示例：

```go
func (j *CleanupJob) Run(
	ctx bindingscheduler.Context,
	jobName bindingscheduler.JobName,
	runID bindingscheduler.RunID,
	at bindingscheduler.ScheduledAt,
) error {
	_ = ctx
	_ = jobName
	_ = runID
	_ = at
	return nil
}
```

## 9. Integrations API 与常见案例

当前 `integrations/*` 统一遵循以下模式之一：

- `ForRoot(...)`
- `ForRoots(...)` / `ForRootMulti(...)`
- `Factory`
- `InjectXxx`
- `XxxRef[NamedMarker]`

### 9.1 Config

包：`github.com/sao-lang/lania-g/integrations/config/v3`

用途：

- 配置加载器注入
- 配置段与配置值自动绑定

```go
type AppConfig struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

func HandleConfig(app config.Section[AppConfig]) error {
	_ = app.Value.Name
	_ = app.Value.Port
	return nil
}
```

### 9.2 Logger

包：`github.com/sao-lang/lania-g/integrations/logger/v3`

用途：

- 默认 logger 注入
- 命名 logger 注入
- 结构化日志输出

```go
func HandleLog(log logger.InjectLogger) error {
	log.Info("request received", logger.String("traceId", "abc"))
	return nil
}
```

命名实例：

```go
type AuditLogger struct{}

func (AuditLogger) LoggerName() string { return "audit" }

func HandleAudit(log logger.LoggerRef[AuditLogger]) error {
	log.Info("audit event")
	return nil
}
```

### 9.3 Cache

包：`github.com/sao-lang/lania-g/integrations/cache/v3`

用途：

- 默认缓存实例注入
- 查询结果缓存
- 函数级缓存包装
- 模板化 key 与批量失效

```go
func HandleCache(c cache.InjectCache) (string, error) {
	return cache.Remember(c.Cache, "user:1", cache.Policy{
		TTL: time.Minute,
	}, func() (string, error) {
		return "alice", nil
	})
}
```

高阶缓存包装：

```go
wrapped, err := cache.DecorateE(
	func(id int) (string, error) {
		return "alice", nil
	},
	cacheInstance,
	cache.DecoratorOptions{
		KeyBuilder: cache.TemplateKeyBuilder("user:{0}"),
		Policy: cache.Policy{
			TTL: time.Minute,
		},
	},
)
if err != nil {
	return err
}

findUser := wrapped.(func(int) (string, error))
_, _ = findUser(1)
```

### 9.4 ORM

包：`github.com/sao-lang/lania-g/integrations/orm/v3`

用途：

- 默认数据源
- 多数据源
- repository 自动绑定

默认库：

```go
func HandleUserRepo(repo orm.InjectRepository[User]) error {
	return repo.Create(&User{Name: "alice"})
}
```

命名库：

```go
type AnalyticsDB struct{}

func (AnalyticsDB) ORMDataSourceName() string { return "analytics" }

func HandleAnalytics(db orm.DataSourceRef[AnalyticsDB]) error {
	_ = db.DB
	return nil
}
```

多数据源初始化：

```go
ormModule, err := orm.ForRoots(
	orm.Config{Name: "default", DB: mainDB},
	orm.Config{Name: "analytics", DB: analyticsDB},
)
```

### 9.5 HTTP Client

包：`github.com/sao-lang/lania-g/integrations/http/v3`

用途：

- 调用外部 HTTP 服务
- 默认 client 与命名 client 并存

```go
func HandleRemote(client httpclient.InjectClient) error {
	_, err := client.Get("/ping")
	return err
}
```

命名实例：

```go
type GitHubClient struct{}

func (GitHubClient) HTTPClientName() string { return "github" }

func HandleGitHub(client httpclient.ClientRef[GitHubClient]) error {
	_, err := client.Get("/users/octocat")
	return err
}
```

### 9.6 gRPC Client

包：`github.com/sao-lang/lania-g/integrations/grpc/v3`

用途：

- 调用外部 gRPC 服务
- 自动注入 client 与连接

```go
func HandleGRPC(
	client grpcclient.InjectClient,
	conn grpcclient.InjectConn,
) error {
	_ = client
	_ = conn.ClientConn
	return nil
}
```

### 9.7 Kafka

包：`github.com/sao-lang/lania-g/integrations/kafka/v3`

用途：

- 注入 Kafka client
- 创建 producer / consumer
- 与 MQ adapter 协同使用

```go
func HandleKafka(client kafka.InjectClient) error {
	producer := client.NewProducer(kafka.ProducerConfig{
		Topic:         "user.created",
		RetryAttempts: 3,
	})
	return producer.PublishValue(stdctx.Background(), map[string]any{
		"id": "1",
	}, kafka.PublishOptions{
		Topic: "user.created",
		Key:   "1",
	})
}
```

### 9.8 Events

包：`github.com/sao-lang/lania-g/integrations/events/v3`

用途：

- 应用内事件总线
- 声明式注册 handler
- 生命周期自动挂载

```go
reg := application.NewRegistry()

type UserEventHandler struct{}

func (h *UserEventHandler) OnUserCreated(ctx context.Context, userID string) error {
	return nil
}

func registerEventHandlers(h *UserEventHandler) {
	events.RegisterOn(reg, "user.created", h, h.OnUserCreated)
}
```

`events.RegisterOnCompat(...)` 等显式全局写入口归为迁移层能力，仅在维护历史全局写路径时再使用。

### 9.9 Swagger

包：`github.com/sao-lang/lania-g/integrations/swagger/v3`

用途：

- 基于 HTTP DSL 自动生成 OpenAPI
- 暴露 `/swagger` 与 `/swagger.json`

```go
swaggerModule, err := swagger.ForRoot(swagger.Config{
	Title:   "lania-g Demo API",
	Version: "1.0.0",
})
_ = swaggerModule
_ = err
```

应用挂桥：

```go
swagger.ServeHTTPBridge(app)
```

### 9.10 Terminus

包：`github.com/sao-lang/lania-g/integrations/terminus/v3`

用途：

- 暴露健康检查
- 输出版本与发布信息

```go
terminusModule, err := terminus.ForRoot(terminus.Config{
	Version:   "1.0.0",
	ReleaseID: "20250411",
})
_ = terminusModule
_ = err
```

应用挂桥：

```go
terminus.ServeHTTPBridge(app, "/health")
```

## 10. 组合示例：从模块到应用再到端口绑定

以下示例展示一个典型 HTTP 应用的组装方式，并体现 `integrations` 的协同使用。

```go
package main

import (
	"time"

	adapterhttp "github.com/sao-lang/lania-g/protocol/http/v3"
	bindinghttp "github.com/sao-lang/lania-g/protocol/http/v3/binding"
	"github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/integrations/cache/v3"
	"github.com/sao-lang/lania-g/integrations/config/v3"
	"github.com/sao-lang/lania-g/integrations/logger/v3"
	"github.com/sao-lang/lania-g/integrations/swagger/v3"
	"github.com/sao-lang/lania-g/integrations/terminus/v3"
)

type UserController struct{}

func (c *UserController) GetUser(
	id bindinghttp.Param[string],
	log logger.InjectLogger,
	cacheInst cache.InjectCache,
) error {
	log.Info("get user")
	_ = cacheInst.Cache.Set("user:last", id.Value)
	return nil
}

func buildRootModule() (module.Module, *UserController, error) {
	configModule, err := config.ForRoot(config.Config{
		Data: map[string]any{
			"app": map[string]any{
				"name": "lania-g-demo",
				"port": 8080,
			},
		},
	})
	if err != nil {
		return nil, nil, err
	}

	loggerModule, err := logger.ForRoot(logger.Config{
		Name:  "default",
		Level: logger.InfoLevel,
	})
	if err != nil {
		return nil, nil, err
	}

	cacheModule, err := cache.ForRoot(cache.Config{
		Type: cache.Memory,
		Name: "default",
	})
	if err != nil {
		return nil, nil, err
	}

	swaggerModule, err := swagger.ForRoot(swagger.Config{
		Title:   "lania-g Demo API",
		Version: "1.0.0",
	})
	if err != nil {
		return nil, nil, err
	}

	terminusModule, err := terminus.ForRoot(terminus.Config{
		Version:   "1.0.0",
		ReleaseID: time.Now().Format("20060102150405"),
	})
	if err != nil {
		return nil, nil, err
	}

	ctrl := &UserController{}

	ctrlProvider, err := di.ProviderFromInstance(ctrl, di.Singleton)
	if err != nil {
		return nil, nil, err
	}

	root := module.CreateModule(
		[]module.Module{configModule, loggerModule, cacheModule, swaggerModule, terminusModule},
		[]*di.Provider{ctrlProvider},
		nil,
		nil,
		nil,
	)
	return root, ctrl, nil
}

func main() {
	root, ctrl, err := buildRootModule()
	if err != nil {
		panic(err)
	}

	httpAdapter := adapterhttp.New()

	app, err := application.NewWithOptions(
		root,
		application.Options{
			Registry: application.NewRegistry(),
		},
		httpAdapter,
	)
	if err != nil {
		panic(err)
	}

	httpAPI := httpAdapter.API().(*adapterhttp.API)
	httpAPI.Controller("/users", ctrl).
		Get("/:id", ctrl.GetUser).
		Build()

	// 以下名称仅表示典型增强点，需由应用自行实现。
	app.UseGlobalMiddlewares(accessLogMiddleware)
	app.UseGlobalGuards(authGuard)
	app.UseGlobalInterceptors(traceInterceptor)
	app.UseGlobalPipes(validationPipe)
	app.UseGlobalFilters(exceptionFilter)

	swagger.ServeHTTPBridge(app)
	terminus.ServeHTTPBridge(app, "/health")

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
```

## 11. 公共包与内部包边界

推荐业务代码直接依赖以下包族：

- `github.com/sao-lang/lania-g/application/v3`
- `github.com/sao-lang/lania-g/kernel/v3/module`
- `github.com/sao-lang/lania-g/kernel/v3/di`
- `github.com/sao-lang/lania-g/protocol/<proto>/v3`
- `github.com/sao-lang/lania-g/protocol/<proto>/v3/binding`
- `github.com/sao-lang/lania-g/integrations/<name>/v3`

这些包承担业务接入、模块装配、协议声明和基础设施接入的稳定职责。

不建议业务直接依赖的内部基础设施包：

- `github.com/sao-lang/lania-g/kernel/v3/compiler`
- `github.com/sao-lang/lania-g/kernel/v3/errors`
- `github.com/sao-lang/lania-g/kernel/v3/graph`
- `github.com/sao-lang/lania-g/kernel/v3/integration`
- `github.com/sao-lang/lania-g/kernel/v3/registry`
- `github.com/sao-lang/lania-g/kernel/v3/runtime`
- `github.com/sao-lang/lania-g/kernel/v3/scanner`
- `github.com/sao-lang/lania-g/kernel/v3/metadata`

这些包更偏向编译、运行时和框架内部协作，不承诺业务侧细粒度直接依赖的稳定性。

### 11.1 业务接入约束

面向业务应用接入时，建议遵守以下约束：

- 优先通过 `application.NewWithOptions(...)` + `application.NewRegistry()` 创建应用实例。
- 优先通过协议模块的实例 API 注册声明，而不是直接依赖旧 compat 入口。
- 不混用全局 `registry.Global()` 与实例级 `application.NewRegistry()`。
- 不把 `kernel/v3/compiler`、`kernel/v3/runtime`、`kernel/v3/registry` 当作常规业务依赖。
- integrations 优先通过 `ForRoot(...)` / `ForRoots(...)` 接入，而不是手工绕过模块边界拼装。

### 11.2 示例约束

- 业务初始化优先参考 [标准接入模板](标准接入模板.md)。
- 如果示例或测试里出现内部基础设施包 import，应理解为框架实现或调试场景，而不是业务默认依赖路径。

## 12. 稳定性约定

- `Must...` API 可以 panic
- 非 `Must...` API 应优先通过 `error` 表达失败
- 实例级 `Registry` 是推荐用法，`registry.Global()` 主要用于兼容默认 DSL 行为
- 小版本内优先保持 `application/v3`、`kernel/v3/module`、`kernel/v3/di`、`protocol/*/v3`、`protocol/*/v3/binding`、`integrations/*/v3` 的源代码兼容
- 最小接入示例优先参考 [标准接入模板.md](标准接入模板.md)
- 专项 integration 细节参见 `auth接入.md`、`mongodb接入.md`、`otel接入.md`、`outbox接入.md`、`resilience接入.md`
