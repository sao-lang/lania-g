---
title: AOP
weight: 4
prev: /docs/reference/registry-compiler-runtime
next: /docs/reference/binding
---

包：`kernel/v3/aop`

## 1. 类型

```go
type MiddlewareFunc      func(ctx *ExecutionContext, next func() error) error
type GuardFunc           func(ctx *ExecutionContext) (bool, error)
type InterceptorFunc     func(ctx *ExecutionContext, next CallHandler) (any, error)
type PipeFunc            func(value any, metadata any) (any, error)
type ExceptionFilterFunc func(exception any, ctx *ExecutionContext) error
```

### CallHandler

```go
type CallHandler interface {
	Handle() (any, error)
}

func NewCallHandler(handler func() (any, error)) CallHandler
```

### ExecutionContext

```go
type ExecutionContext struct {
	Context        context.Context
	HandlerContext any
	Handler        any
	Class          any
	Protocol       string
	RouteKey       string
}

func (ctx *ExecutionContext) GetRequest() any
func (ctx *ExecutionContext) GetResponse() any
```

`HandlerContext` 通常是 `*runtime.HandlerContext`，因此可以直接读取
`Request` / `Response` / metadata。

## 2. 执行顺序

```text
Middleware (外 → 内)
  Guard (允许 / 拒绝)
    Interceptor (外 → 内)
      Pipe (入参转换)
        Handler
      Pipe (返回值转换)
    Interceptor
  Guard
Middleware
ExceptionFilter 捕获整条链路的错误
```

- 任一 Guard 返回 `false` → 拒绝
- 任一环节返回 error → 交给 Filter（若有），否则向上抛
- 参数绑定/校验失败发生在 interceptor 之前，同样会交给 Filter（Executor 的 `HandleError`）
- handler panic → 被 recover 为 `ErrExecutionFailed`

{{< callout type="warning" >}}
`ValidationPipe` 那一类错误（`Body[T]` 的 `validate` 标签、`Param[int]` 转换失败）
不会进入 Interceptor 链。想让它们也走统一响应形态，必须用 ExceptionFilter：
filter 返回 nil 表示已消费，此时由 filter 自己设置 `ctx.Response.Status` / `Body`。
{{< /callout >}}

## 3. 挂载级别

| 级别 | 生效范围 | 声明方式 |
|------|----------|----------|
| 应用级全局 | 所有协议的所有路由 | `app.UseGlobalMiddlewares(...)` 等五个方法 |
| 模块级（全局贡献） | 并入全局链，按模块加载顺序 | `module.Metadata.AOP` 或 `Init()` 中 `ctx.Use(...)` |
| 模块级（路由范围） | 命中的方法 / 路径 | 模块实现 `Configure(consumer aop.MiddlewareConsumer)` |
| 控制器 / 服务 / 网关级 | 该声明单元下的全部路由 | 第一条路由之前的 `Use*`（HTTP/gRPC/GraphQL/WS） |
| 路由 / 方法 / 字段级 | 单条路由 | 动词之后的链式 `Use*` |
| 参数级 | 单个参数 | `UseParamPipes(index, pipes...)`（**仅 Pipe**） |

合并顺序是 **全局 → 范围化 → 局部**，越靠后越靠近 handler。
HTTP/gRPC/GraphQL/WS 在编译期就把这三段拍平成 `HandlerMeta.CompiledAOP`，
运行期不再二次合并。

`Use*` 的参数是 AOP 接口（`aop.Guard`、`aop.Interceptor`、`aop.Middleware`、`aop.Pipe`、
`aop.ExceptionFilter`），`app.UseGlobal*` 与各协议 DSL 用的是同一套类型。因此：

```go
func timingInterceptor(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) { ... }

// 裸函数需要显式转换（类型不符会在编译期报错，不会静默丢弃）
r.Get("/users", ctrl.List).UseInterceptors(aop.InterceptorFunc(timingInterceptor))
app.UseGlobalInterceptors(aop.InterceptorFunc(timingInterceptor))
```

## 4. 声明形态

DSL 的 `Use*` 方法参数是**接口类型**（`aop.Guard`、`aop.Interceptor`、`aop.Middleware`、
`aop.Pipe`、`aop.ExceptionFilter`），所以下面四种写法都能直接传入：

| # | 形态 | Middleware | Guard | Interceptor | Pipe | Filter |
|---|------|-----------|-------|-------------|------|--------|
| 1 | 接口实现 | `Use(ctx, next)` | `CanActivate` | `Intercept` | `Transform` | `Catch` |
| 2 | 具名函数类型 | `aop.MiddlewareFunc` | `aop.GuardFunc` | `aop.InterceptorFunc` | `aop.PipeFunc` | `aop.ExceptionFilterFunc` |
| 3 | 同签名裸函数 | 需包 `aop.MiddlewareFunc(...)` | 需包 `aop.GuardFunc(...)` | 需包 `aop.InterceptorFunc(...)` | 需包 `aop.PipeFunc(...)` | 需包 `aop.ExceptionFilterFunc(...)` |
| 4 | 延迟构造器 | `aop.MiddlewareFactory(ctor)` | `aop.GuardFactory(ctor)` | `aop.InterceptorFactory(ctor)` | `aop.PipeFactory(ctor)` | `aop.FilterFactory(ctor)` |

因为参数是接口而不是 `any`，第 3 种形态需要显式转换：

```go
r.Get("/users", c.List).UseGuards(aop.GuardFunc(func(ctx *aop.ExecutionContext) (bool, error) {
	return true, nil
}))
```

第 4 种形态用于"增强器依赖 DI 实例、不能提前构造"的场景：

```go
app.UseGlobalGuards(aop.GuardFactory(func() aop.Guard {
	return &AuthGuard{Users: container.MustResolve(di.TokenFor[*UserRepo]())}
}))
```

- 构造器在**首次调用时**实例化并缓存，之后复用同一个实例。
- `app.UseGlobal*` 与 `Use*` 接收同一套接口类型，因此 `aop.GuardFactory` 这类包装产物在两边都能直接用。
- **类型不符会在编译期报错**，不存在"传错类型被静默丢弃"的可能。

## 5. 内置 Pipe

`kernel/v3/aop` 自带：

```go
NewParseIntPipe()    *ParseIntPipe
NewParseBoolPipe()   *ParseBoolPipe
NewParseFloatPipe()  *ParseFloatPipe
NewParseArrayPipe()  *ParseArrayPipe
NewParseUUIDPipe()   *ParseUUIDPipe
NewDefaultValuePipe(defaultValue any) *DefaultValuePipe
NewValidationPipe()  *ValidationPipe
```

`kernel/v3/aop/pipes` 补充常用清洗/格式校验：

```go
pipes.NewTrimSpacePipe()            // string / []string / 结构体字符串字段去空白
pipes.NewParseTimePipe(layout)      // RFC3339、2006-01-02 15:04:05、2006-01-02 自动尝试
pipes.NewParseEnumPipe("admin", "member")  // 默认大小写不敏感
pipes.NewRequiredPipe()             // 拒绝 nil / 空串 / 空集合 / 零值
```

全局注册只接受函数形态，对象需要 `aop.WrapPipe`：

```go
app.UseGlobalPipes(aop.WrapPipe(pipes.NewTrimSpacePipe()))
```

## 6. Guard 接口

```go
type Guard interface {
	CanActivate(ctx *ExecutionContext) (bool, error)
}

type GuardConstructor func() Guard
func WrapGuard(guard Guard) GuardFunc
func (f GuardFunc) CanActivate(ctx *ExecutionContext) (bool, error)
```

`GuardFunc` 本身实现了 `Guard`，因此可以直接传给 `UseGuards`。

## 7. 异常过滤器

```go
type ExceptionFilter interface {
	Catch(exception any, ctx *ExecutionContext) error
}

type ExceptionFilterConstructor func() ExceptionFilter
```

`ExceptionFilterFunc` 也可直接作为过滤器使用。

返回 `nil` 表示错误已被消费。

## 8. HTTP 异常

```go
type HttpException struct {
	Status  int
	Message string
	Cause   error
	Headers map[string]string // 405 的 Allow、429 的 Retry-After 等
	Code    string            // 业务错误码
	Details any               // 结构化细节（字段级校验错误等）
}

func (e *HttpException) Unwrap() error
func FromError(err error) (*HttpException, bool)
func StatusFromKind(kind kerrors.Kind) int
```

内置构造器覆盖常见 4xx / 5xx（完整列表见 [错误模型](errors)）：

```go
BadRequestException / UnauthorizedException / PaymentRequiredException
ForbiddenException / NotFoundException / MethodNotAllowedException
NotAcceptableException / RequestTimeoutException / ConflictException
GoneException / LengthRequiredException / PreconditionFailedException
PayloadTooLargeException / URITooLongException / UnsupportedMediaTypeException
RangeNotSatisfiableException / ExpectationFailedException
UnprocessableEntityException / LockedException / FailedDependencyException
UpgradeRequiredException / PreconditionRequiredException / TooManyRequestsException
RequestHeaderFieldsTooLargeException / UnavailableForLegalReasonsException
InternalServerErrorException / NotImplementedException / BadGatewayException
ServiceUnavailableException / GatewayTimeoutException / HTTPVersionNotSupportedException
InsufficientStorageException / LoopDetectedException / NetworkAuthenticationRequiredException
```

带额外参数的：

```go
MethodNotAllowedException("", "GET", "POST")  // 自动写 Allow
TooManyRequestsException("", 3*time.Second)   // 自动写 Retry-After
```

各协议会把 `HttpException` 映射到自己的错误表达：

| 协议 | 映射 |
|------|------|
| HTTP | 状态码 + JSON |
| gRPC | gRPC status |
| GraphQL | errors 数组 |
| WS | `<event>Error` |
| MQ | Nack |

## 9. Middleware 消费者（按路由范围）

模块实现 `Configure(consumer aop.MiddlewareConsumer)`，即可把中间件绑定到指定路由范围：

```go
type AccessScopeModule struct{ *module.BaseModule }

func NewAccessScopeModule() *AccessScopeModule {
	return &AccessScopeModule{BaseModule: module.New(module.Metadata{})}
}

func (m *AccessScopeModule) Configure(consumer aop.MiddlewareConsumer) {
	consumer.Apply(auditMiddleware("users")).
		ForRoutes("/users/*").
		Exclude("GET /users")

	consumer.Apply(auditMiddleware("admin")).
		ForRoutes("GET /admin/*")
}
```

把该模块放进根模块的 `Imports` 即可，无需手工调用任何注册方法。

```go
type MiddlewareConsumer interface {
	Apply(middlewares ...Middleware) MiddlewareRouteConfigurator
}

type MiddlewareRouteConfigurator interface {
	ForRoutes(routes ...string) MiddlewareRouteConfigurator
	Exclude(routes ...string) MiddlewareRouteConfigurator
}
```

- **每次 `Apply` 是一条独立绑定**，不同中间件可以有不同的路由范围。
- `Apply` 接受的类型与 `UseMiddlewares` 一致，都是 `aop.Middleware`；普通函数要写成 `aop.MiddlewareFunc(fn)`，类型不符会在编译期报错。
- `Exclude` 优先级高于 `ForRoutes`；`ForRoutes` 为空时默认对所有路由生效。
- 生效顺序：全局中间件 → 范围化中间件（按模块加载顺序）→ 路由级中间件。

### 路由规则

规则既可只写路径，也可带方法前缀：

| 写法 | 含义 |
|------|------|
| `"/users/*"` | 前缀匹配：命中 `/users` 与 `/users/42/posts`，不会命中 `/usersettings` |
| `"/users/:id"` / `"/users/{id}"` | 单段参数匹配：命中 `/users/42`，不命中 `/users/42/posts` |
| `"*"` | 匹配全部路由 |
| `"GET /users"` | 仅在 `GET` 方法上生效 |
| `"ALL /admin/*"` | 方法不限，等价于只写路径 |

匹配按路径段进行，不是字符串前缀，因此 `/users` 不会误命中 `/usersettings`。

### 各协议的路由标识

范围匹配使用协议的“原生路径”，不同协议填写的字段如下：

| 协议 | 匹配路径 | 方法 |
|------|----------|------|
| HTTP | `/users/:id` | `GET` / `POST` / … |
| gRPC | `/pkg.Service/Method` | 空（规则不要写方法前缀） |
| GraphQL | `Query.user` 或 `<Resolver>.field` | 空 |
| WebSocket | `/chat/message` | 空 |
| MQ | topic | 空 |
| Scheduler | job name | 空 |

## 10. 内置增强器（kernel/v3/runtime/enhancers）

`aop` 包只放抽象，不能依赖 `runtime`。需要读取 `*runtime.HandlerContext` 的通用增强器
统一放在 `kernel/v3/runtime/enhancers`，四个协议共用：

```go
import "github.com/sao-lang/lania-g/kernel/v3/runtime/enhancers"

app.UseGlobalMiddlewares(
	enhancers.Recovery(func(recovered any) { log.Printf("panic: %v", recovered) }),
	enhancers.RequestID("X-Request-Id", nil),
	enhancers.AccessLog(func(e enhancers.AccessEntry) {
		log.Printf("%s %s %s %d %s rid=%s", e.Protocol, e.Method, e.Path, e.Status, e.Duration, e.RequestID)
	}),
	enhancers.Timeout(5*time.Second),
)
app.UseGlobalFilters(enhancers.ErrorEnvelope(nil))
```

| 增强器 | 作用 |
|--------|------|
| `Recovery(reporter)` | 捕获 panic → 500，并回调上报 |
| `RequestID(header, gen)` | 透传/生成请求 ID，写入上下文、请求头、响应头 |
| `AccessLog(logger)` | 结构化访问日志（协议、路由、状态码、耗时、请求 ID） |
| `Timeout(d)` | 设置 context 超时，超时映射为 504（协作式，handler 需尊重 ctx） |
| `ErrorEnvelope(body)` | 把任意错误写成 `runtime.Response`，统一错误形态 |

`AccessLog` 的 `AccessEntry`：

```go
type AccessEntry struct {
	Protocol, ModuleKey, RouteKey, RequestID string
	Method, Path                              string
	Status                                    int
	Duration                                  time.Duration
	Err                                       error
}
```

## 11. 常见错误

| 现象 | 原因 |
|------|------|
| 中间件执行两次 | `CompiledAOP` 与全局列表被同时应用 |
| Guard 不生效 | 传了不支持的签名；现在会直接编译报错 `*aop.ErrUnsupported` |
| `Configure` 没被调用 | 模块没有实现 `Configure(aop.MiddlewareConsumer)`，或没挂进 `Imports` |
| 范围化中间件不生效 | 规则与路由路径不匹配（注意 gRPC 的 `/pkg.Service/Method` 形式） |
| Interceptor 吞掉响应 | 返回了 `nil, nil` |
| Filter 未触发 | Filter 只处理 error 路径，业务返回成功不会触发 |
