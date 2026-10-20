---
title: HTTP
weight: 6
prev: /docs/reference/binding
next: /docs/reference/grpc
---

包：`github.com/sao-lang/lania-g/protocol/http/v3`

## 1. 模块入口

```go
func Module(cfg Config) module.Module
```

`Config` 字段：

| 字段 | 说明 |
|------|------|
| `Addr` | 监听地址，例如 `":8080"`（必填） |
| `BasePath` | 统一路由前缀 |
| `Validator` | `true` 时启用 go-playground/validator v10 |
| `ValidatorImpl` | 自定义校验器（优先于 `Validator`） |
| `Renderer` | 模板渲染器 |
| `MaxBodyBytes` | 请求体上限（默认 10MB） |
| `PreRouting` | 路由匹配**之前**的传输层中间件（`func(*httpbinding.HttpContext) error`，gin 风格） |
| `CORS` / `Helmet` | 内建 CORS / 安全响应头 |
| `NotFound` | 未命中路由兜底 handler |

```go
root := module.New(module.Metadata{
	Imports: []module.Module{
		http.Module(http.Config{Addr: ":8080", Validator: true}),
	},
	Controllers: []any{&UserController{}},
})
```

### 命令式入口

```go
func New(cfg Config) *Adapter            // 构造未挂载的 adapter（EnableCors/UsePreRouting 等链式 API）
func (a *Adapter) Addr() string          // 当前监听地址（app.Listen 可能已覆盖）
func (a *Adapter) EnableCors(*CorsConfig) *Adapter
func (a *Adapter) EnableHelmet(*HelmetConfig) *Adapter
func (a *Adapter) UsePreRouting(mws ...func(*httpbinding.HttpContext) error) *Adapter

// 复用框架统一错误响应（自定义 transport / 中间件用）
func WriteError(w http.ResponseWriter, r *http.Request, ctx *runtime.HandlerContext, err error)
```

{{< callout type="info" >}}
只配置了 `CORS` / `Helmet` 时用 `Module` 即可；`New` 适合"先构造、再链式追加配置"的场景。
{{< /callout >}}

## 2. 控制器与路由声明

```go
type Controller interface {
	Declare(*Routes)
}
```

控制器实现该接口后，模块加载阶段会自动收集其路由，无需手动 Build：

```go
func (c *UserController) Declare(r *http.Routes) {
	r.Prefix("/users").
		UseGuards(auth.RequireAuthenticated()).
		Get("/:id", c.Get).
			Summary("Get User").
			Tags("users").
			Security("bearerAuth").
			ErrorResponse(404, "用户不存在").
		Post("", c.Create).
			Status(201)
}
```

### Routes 方法

作用域方法（第一条路由之前，作用于整个控制器）：

```go
Prefix(prefix string) *Routes
UseGuards(guards ...aop.Guard) *Routes
UseInterceptors(interceptors ...aop.Interceptor) *Routes
UseMiddlewares(middlewares ...aop.Middleware) *Routes
UsePipes(pipes ...aop.Pipe) *Routes
UseFilters(filters ...aop.ExceptionFilter) *Routes
UseParamPipes(paramIndex int, pipes ...aop.Pipe) *Routes
```

参数是接口类型，所以普通函数需显式包装：`aop.GuardFunc(fn)`、`aop.InterceptorFunc(fn)`、
`aop.MiddlewareFunc(fn)`、`aop.PipeFunc(fn)`、`aop.ExceptionFilterFunc(fn)`；
需要延迟构造（依赖 DI 实例）时用 `aop.GuardFactory(newXxx)` 等工厂包装器。

动词方法（开始一条新路由）：

```go
Get / Post / Put / Delete / Patch / Head / Options / All(path string, handler any) *Routes
```

修饰方法（作用于当前路由）：

| 方法 | 作用 |
|------|------|
| `Summary(string)` | 文档摘要 |
| `Description(string)` | 文档描述 |
| `Tags(...string)` | 文档分组 |
| `Security(name, scopes...)` | 安全方案（仅文档） |
| `Status(code int)` | 固定成功状态码 |
| `Header(k, v)` / `Headers(map)` | 固定响应头 |
| `ResponseBody(example)` | 响应示例 |
| `ResponseEnvelope(envelope, field)` | 响应包裹类型 |
| `ErrorResponse(status, desc)` | 错误响应文档 |
| `HideFromDocs()` | 从文档隐藏 |
| `Redirect(url, status...)` | 重定向 |
| `Render(template)` | 模板渲染 |

{{< callout type="warn" >}}
修饰方法必须紧跟某条路由声明；出现在任何动词之前会被记为声明错误，
并在 `application.New` 阶段返回。
{{< /callout >}}

## 3. 原生 handler 挂载

需要挂载 Swagger、健康检查、静态资源这类非 runtime 路由时，
模块实现 `adapter.HTTPMounterProvider`：

```go
func (m *MyModule) MountHTTP(req adapter.HTTPMountRequest) error {
	req.Mounter.Handle("/metrics", metricsHandler)
	req.Mounter.Static("/assets", "./public")
	return nil
}
```
