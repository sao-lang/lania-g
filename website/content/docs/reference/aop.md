---
title: AOP
weight: 4
prev: /docs/reference/registry-compiler-runtime
next: /docs/reference/binding
---

包：`kernel/v3/aop`

## 1. 类型

```go
type MiddlewareFunc func(ctx *ExecutionContext, next CallHandler) (any, error)
type GuardFunc      func(ctx *ExecutionContext) (bool, error)
type InterceptorFunc func(ctx *ExecutionContext, next CallHandler) (any, error)
type PipeFunc       func(value any, metadata *ArgumentMetadata) (any, error)
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

| 级别 | 方式 |
|------|------|
| 全局 | `app.UseGlobalInterceptors(...)` 等 |
| 控制器 | `r.UseGuards(...)`（`Routes` 作用域，HTTP/gRPC/WS/…） |
| 路由/字段 | 链式 `Get(...).UseInterceptors(...)` |
| 参数级 Pipe | `UseParamPipes(index, pipes...)` |

`Use*` 既可以接收 `aop.GuardFunc` / `aop.InterceptorFunc` 这类具名类型，
也可以直接接收**同签名的裸函数**（编译器会自动转换），因此下面两种写法等价：

```go
func timingInterceptor(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) { ... }

r.Get("/users", ctrl.List).UseInterceptors(timingInterceptor)
r.Get("/users", ctrl.List).UseInterceptors(aop.InterceptorFunc(timingInterceptor))
```

## 4. 内置 Pipe

```go
func NewDefaultValuePipe(defaultValue any) *DefaultValuePipe
```

`Transform(value, metadata)`：当 `value` 为 nil 或零值时替换为默认值。

## 5. Guard 接口

```go
type Guard interface {
	CanActivate(ctx *ExecutionContext) (bool, error)
}

type GuardConstructor func() Guard
func WrapGuard(guard Guard) GuardFunc
func (f GuardFunc) CanActivate(ctx *ExecutionContext) (bool, error)
```

`GuardFunc` 本身实现了 `Guard`，因此可以直接传给 `UseGuards`。

## 6. 异常过滤器

```go
type ExceptionFilter interface {
	Catch(exception any, ctx *ExecutionContext) error
}

type ExceptionFilterConstructor func() ExceptionFilter
```

`ExceptionFilterFunc` 也可直接作为过滤器使用。

返回 `nil` 表示错误已被消费。

## 7. HTTP 异常

```go
type HttpException struct {
	Status  int
	Message string
}

func BadRequestException(msg string) *HttpException
func UnauthorizedException(msg string) *HttpException
func ForbiddenException(msg string) *HttpException
func NotFoundException(msg string) *HttpException
func ConflictException(msg string) *HttpException
func GoneException(msg string) *HttpException
func BadGatewayException(msg string) *HttpException
func GatewayTimeoutException(msg string) *HttpException
func InternalServerErrorException(msg string) *HttpException
```

各协议会把 `HttpException` 映射到自己的错误表达：

| 协议 | 映射 |
|------|------|
| HTTP | 状态码 + JSON |
| gRPC | gRPC status |
| GraphQL | errors 数组 |
| WS | `<event>Error` |
| MQ | Nack |

## 8. Middleware 消费者

```go
type MiddlewareConsumer interface {
	Apply(middlewares ...Middleware) MiddlewareConsumer
	ForRoutes(routes ...string) MiddlewareConsumer
	Exclude(routes ...string) MiddlewareConsumer
	GetMiddlewares() []Middleware
	ShouldApply(path string) bool
}
```

用于按路径选择性地应用 middleware。

## 9. 常见错误

| 现象 | 原因 |
|------|------|
| 中间件执行两次 | `CompiledAOP` 与全局列表被同时应用 |
| Guard 不生效 | 传的是未包装的 `GuardFunc`（需实现 `CanActivate`，`GuardFunc` 已实现） |
| Interceptor 吞掉响应 | 返回了 `nil, nil` |
| Filter 未触发 | Filter 只处理 error 路径，业务返回成功不会触发 |
