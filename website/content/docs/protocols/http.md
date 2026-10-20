---
title: HTTP
weight: 1
prev: /docs/core-concepts/errors-lifecycle
next: /docs/protocols/grpc
---

包：`github.com/sao-lang/lania-g/protocol/http/v3`

## 1. 创建协议模块

```go
httpModule := http.Module(http.Config{
	Addr:         ":8080",
	BasePath:     "/api",
	MaxBodyBytes: 10 << 20,
	Validator:    true,
})
```

常用配置：

| 字段 | 作用 |
|------|------|
| `Addr` | 监听地址（必填） |
| `BasePath` | 统一路由前缀 |
| `MaxBodyBytes` | 请求体大小限制 |
| `Validator` / `ValidatorImpl` | 启用 `validate:"..."` 校验 |
| `CORS` / `Helmet` | 内建 CORS / 安全响应头 |
| `Middlewares` | adapter 层中间件 |
| `NotFound` | 未命中路由的兜底 handler |

把模块放进根模块的 `Imports` 即可；适配器由应用在装配阶段自动发现。

### CORS / Helmet / Body 限制

这三项都是 **传输层能力**，在 `Config` 里一次性配置，执行于路由匹配之前
（mounted handler 如 `/docs` 同样生效）：

```go
http.Module(http.Config{
	Addr: ":8080",
	CORS: &http.CorsConfig{
		AllowOrigins:     []string{"https://app.example.com"},
		AllowOriginFunc:  func(origin string) bool { return strings.HasSuffix(origin, ".example.com") },
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		ExposeHeaders:    []string{"X-Request-Id"},
		AllowCredentials: true,
		MaxAge:           3600,
	},
	Helmet:       http.DefaultHelmetConfig(),
	MaxBodyBytes: 10 << 20, // 超出返回 413（统一错误信封）
})
```

CORS 行为约定：

| 场景 | 行为 |
|------|------|
| `AllowOrigins: ["*"]` 且未开 credentials | 返回 `Access-Control-Allow-Origin: *` |
| `AllowOrigins: ["*"]` 且开 credentials | **回显请求的实际 origin**（规范禁止 `*` + credentials） |
| 命中 `AllowOriginFunc` | 回显请求的实际 origin |
| `AllowHeaders` 为空 | 不下发 `Allow-Headers`（不会回显客户端值） |
| `AllowAllHeaders: true` | 显式开启后才回显客户端请求头 |
| 任意 CORS 响应 | 都会带 `Vary: Origin`；preflight 额外带两个 `Vary` |

preflight（`OPTIONS` + `Access-Control-Request-Method`）在 adapter 内直接返回 `204`，
不会进入路由与业务 handler。CORS **不做成 AOP Middleware**，因为 preflight 必须在
路由与参数绑定之前短路。

## 2. 路由声明

```go
func (c *UserController) Declare(r *http.Routes) {
	r.Prefix("/users").
		UseGuards(auth.RequireAuthenticated()).
		Get("", c.List).
			Summary("List Users").
			Description("分页查询用户列表").
			Tags("users").
			Security("bearerAuth").
			ResponseEnvelope(apiEnvelope{}, "data").
			ErrorResponse(401, "未登录").
		Get("/:id", c.Get).
		Post("", c.Create).
			Status(201).
		Put("/:id", c.Update).
		Delete("/:id", c.Delete)
}
```

规则：

- 第一个动词之前的 `Prefix` / `Use*` 作用于整个控制器
- 动词方法开始一条新路由
- 其余修饰方法作用于当前路由
- 不需要 `Build()`：声明阶段由模块树自动收集

## 3. 参数绑定

```go
type getUserArgs struct {
	ID      httpbinding.Param[int]     `param:"id" required:"true"`
	TraceID httpbinding.Header[string] `header:"X-Trace-Id"`
}
```

完整 wrapper 列表见 [Binding](../core-concepts/binding)。

## 4. 校验

```go
httpModule := http.Module(http.Config{Addr: ":8080", Validator: true})

type createUserRequest struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gt=0"`
}
```

失败时返回：

```json
{
  "error": "validation failed on Email (email)",
  "kind": "Validation",
  "meta": { "errors": [{ "field": "Email", "tag": "email" }] }
}
```

## 5. 响应控制

```go
func (c *UserController) Create(args createUserArgs) (any, error) {
	args.Ctx.Status(http.StatusCreated)
	args.Ctx.Header("X-Trace-Id", "abc")
	return item, nil
}
```

## 6. Swagger

```go
swaggerModule := swagger.Module(swagger.Config{
	Title:                "Demo API",
	Version:              "1.0.0",
	BearerAuth:           []string{"bearerAuth"},
	DefaultErrorResponse: "请求失败",
}, &swagger.UIConfig{
	SpecURL:    "/docs/openapi.json",
	SwaggerURL: "/docs",
	RedocURL:   "/redoc",
})
```

把 `swaggerModule` 放进根模块的 `Imports` 即可：OpenAPI 在声明阶段之后生成，
UI/spec 端点由模块自己挂载，不需要手动调用 `BuildFromHTTPRegistry` / `ServeHTTPBridge`。

## 7. 静态资源与 SSE

```go
// 模块实现 adapter.HTTPMounterProvider 即可挂载原生 handler
func (m *MyModule) MountHTTP(req adapter.HTTPMountRequest) error {
	req.Mounter.Static("/assets", "./public")
	req.Mounter.Handle("/feed", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// 持续写入 event-stream
	}))
	return nil
}
```

## 8. 监听地址

监听地址只来自 `http.Config{Addr}`：

```go
http.Module(http.Config{Addr: ":8080"})
```

多协议同进程时，每个协议模块各自声明自己的地址（例如 HTTP `:8080`、
WebSocket `:8081`、gRPC `:50051`），由 `app.Run()` 统一启动。
