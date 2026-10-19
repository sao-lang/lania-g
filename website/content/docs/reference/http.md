---
title: HTTP
weight: 6
prev: /docs/reference/binding
next: /docs/reference/grpc
---

包：`github.com/sao-lang/lania-g/protocol/http/v3`

## 1. Adapter

```go
func New(addr ...string) *Adapter   // 无参=共享监听；带参=独立监听
func Listen(addr string) *Adapter   // Deprecated
```

| 方法 | 说明 |
|------|------|
| `WithBasePath(prefix)` | 统一前缀（请求路径会先剥离） |
| `WithMaxBodyBytes(n)` | body 上限（默认 10MB） |
| `WithRenderer(r)` | 自定义响应渲染器 |
| `WithValidator(v)` | 自定义校验器 |
| `WithValidatorV10()` | `validate` 标签校验 |
| `EnableCors(cfg)` | CORS（nil 用默认） |
| `EnableHelmet(cfg)` | 安全响应头 |
| `ServeStatic(prefix, root)` | 静态目录 |
| `MountHandler(pattern, h)` | 挂载原生 handler |
| `WithNextHandler(h)` | 未命中路由兜底 |
| `UseAfter(mws...)` | 路由执行后的 middleware |
| `SetGlobalPrefix(prefix)` | 兼容命名，等价 basePath |
| `RequiresSharedListen()` | 是否依赖共享监听 |
| `ConfigureSharedListen(addr)` | 注入共享地址 |

`New(addr)` 与 `New()` 的差异：

| 模式 | 启动 |
|------|------|
| `New()` | `app.Run()` |
| `New(":8080")` | `app.Run()` |

## 2. API 与 Builder

```go
func NewAPI(reg *registry.Registry, mountHandler func(string, http.Handler)) *API
func (api *API) Controller(prefix string, controller any) *ControllerBuilder
func (api *API) MountHandler(prefix string, handler http.Handler)
```

### ControllerBuilder

```go
Get/Post/Put/Patch/Delete/Head/Options/All(path string, handler any) *RouteBuilder
UseMiddlewares/UseGuards/UseInterceptors/UsePipes/UseFilters(items ...any) *ControllerBuilder
UseParamPipes(paramIndex int, pipes ...any) *ControllerBuilder
Build() []*RouteDefinition
BuildE() ([]*RouteDefinition, error)
Err() error
```

### RouteBuilder

| 方法 | 作用 |
|------|------|
| `Summary(string)` | 文档摘要 |
| `Description(string)` | 文档描述 |
| `Tags(...string)` | 文档分组 |
| `Security(name, scopes...)` | 安全方案 |
| `ResponseBody(example)` | 响应示例 |
| `ResponseEnvelope(envelope, field)` | 响应包裹类型 |
| `ErrorResponse(status, desc)` | 错误响应文档 |
| `StatusCode(code)` | 固定状态码（终结方法） |
| `Header(k, v)` / `Headers(map)` | 固定响应头 |
| `Redirect(url, status...)` | 重定向（终结方法） |
| `Render(template)` | 模板渲染（终结方法） |
| `HideFromDocs()` | 不在文档中展示 |
| `UseMiddlewares/Guards/Interceptors/Pipes/Filters(...)` | 路由级 AOP |
| `UseParamPipes(index, ...)` | 参数级 Pipe |
| `Build()/BuildE()` | 完成声明 |

> 「终结方法」会结束当前路由的链式调用，后续需要重新 `Get/Post`。

## 3. 路由匹配

- 静态段优先于参数段
- `:id` 形式声明 path 参数
- `BuildE()` 会校验重复 routeKey 与 handler 签名

## 4. 请求绑定

见 [Binding 矩阵](binding#http)。

```go
type getUserArgs struct {
	ID   httpbinding.Param[int]     `param:"id" required:"true"`
	Trace httpbinding.Header[string] `header:"X-Trace-Id"`
}
```

## 5. 响应

### 直接返回

```go
func (c *C) Get() (any, error) { return user, nil }
```

默认 JSON 编码；`map`/struct 均可。

### 控制状态码与头

```go
args.Ctx.Status(http.StatusCreated)
args.Ctx.Header("X-Request-Id", id)
```

### 统一包裹

```go
type envelope struct {
	Msg string `json:"msg"`
	Code int `json:"code"`
	Data any `json:"data"`
	Success bool `json:"success"`
}
```

用 Interceptor 包裹即可，同时可在 DSL 中用 `ResponseEnvelope(envelope{}, "data")` 生成文档。

### 重定向与渲染

```go
.Get("/old", ctrl.Old).Redirect("/new", http.StatusMovedPermanently)
.Get("/page", ctrl.Page).Render("page.html")
```

## 6. CORS / Helmet

```go
httpAdapter.EnableCors(nil)     // 默认允许全部来源
httpAdapter.EnableHelmet(nil)   // 默认安全头
```

自定义：

```go
httpAdapter.EnableCors(&httpadapter.CorsConfig{
	AllowOrigins: []string{"https://example.com"},
	AllowMethods: []string{"GET", "POST"},
	AllowHeaders: []string{"Content-Type", "Authorization"},
	AllowCredentials: true,
	MaxAge: 3600,
})
```

## 7. 静态资源与挂载

```go
httpAdapter.ServeStatic("/assets", "./public")

httpAdapter.MountHandler("/metrics", promhttp.Handler())

httpAdapter.WithNextHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}))
```

## 8. Swagger

```go
swaggerModule, _ := swagger.Module(
	swagger.Config{Title: "Demo API", Version: "1.0.0"},
	&swagger.UIConfig{
		Title: "Docs", SpecURL: "/docs/openapi.json",
		SwaggerURL: "/docs", RedocURL: "/redoc",
	},
)

builder, _ := module.GetByType[*swagger.Builder](app.ModuleRef())
builder.AddBearerAuth("bearerAuth")
builder.SetDefaultErrorResponse("Request failed", envelope{}, 400, 500)
swagger.BuildFromHTTPRegistry(builder, reg)
swagger.ServeHTTPBridge(app)
```

## 9. 错误映射

| KernelError.Kind | HTTP 状态 |
|------------------|-----------|
| `KindBinding` / `KindValidation` / `KindDI` | 400 |
| `KindUnauthorized` | 401 |
| `KindForbidden` | 403 |
| `KindRouteNotFound` | 404 |
| `KindExecution` / 其他 | 500 |

`*aop.HttpException` 会优先使用其 `Status`（例如 `aop.ConflictException` → 409）。

默认响应体：

```json
{
  "error": "validation failed on Email (email)",
  "status": 400,
  "path": "/users",
  "kind": "Validation",
  "meta": { "errors": [] }
}
```

要改格式就挂一个 `ExceptionFilter`（`aop.ExceptionFilterFunc`），它会在写响应前接管错误。
