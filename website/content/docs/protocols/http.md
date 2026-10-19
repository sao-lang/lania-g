---
title: HTTP
weight: 1
prev: /docs/core-concepts/errors-lifecycle
next: /docs/protocols/grpc
---

包：`github.com/sao-lang/lania-g/protocol/http/v3`

## 1. 创建 adapter

```go
httpAdapter := httpadapter.New()             // 共享监听（跟随 app.Listen）
httpAdapter := httpadapter.New(":8080")      // 独立监听（跟随 app.Run）

httpAdapter := httpadapter.New().
	WithBasePath("/api").
	WithMaxBodyBytes(10 << 20).
	WithValidatorV10()
```

常用配置：

| 方法 | 作用 |
|------|------|
| `WithBasePath(prefix)` | 统一路由前缀 |
| `WithMaxBodyBytes(n)` | 请求体大小限制 |
| `WithValidator(v)` | 自定义校验器 |
| `WithValidatorV10()` | 启用 `validate:"..."` 校验 |
| `EnableCors(cfg)` | 内建 CORS |
| `EnableHelmet(cfg)` | 安全响应头 |
| `ServeStatic(prefix, root)` | 静态资源 |
| `MountHandler(pattern, h)` | 挂载原生 `http.Handler` |
| `WithNextHandler(h)` | 未命中路由的兜底 handler |

## 2. 路由 DSL

```go
httpAPI := httpAdapter.API().(*httpadapter.API)

httpAPI.Controller("/users", ctrl).
	Get("", ctrl.List).
	Summary("List Users").
	Description("分页查询用户列表").
	Tags("users").
	Security("bearerAuth").
	ResponseEnvelope(apiEnvelope{}, "data").
	ErrorResponse(401, "未登录").
	Get("/:id", ctrl.Get).
	Post("", ctrl.Create).
	StatusCode(201).
	Put("/:id", ctrl.Update).
	Delete("/:id", ctrl.Delete).
	BuildE()
```

- `Build()`：忽略错误（兼容写法）
- `BuildE()`：声明非法时返回错误（推荐）

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
httpAdapter := httpadapter.New().WithValidatorV10()

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
swaggerModule, _ := swagger.Module(swagger.Config{
	Title: "Demo API", Version: "1.0.0",
}, &swagger.UIConfig{
	SpecURL:    "/docs/openapi.json",
	SwaggerURL: "/docs",
	RedocURL:   "/redoc",
})

builder, _ := module.GetByType[*swagger.Builder](app.ModuleRef())
builder.AddBearerAuth("bearerAuth")
swagger.BuildFromHTTPRegistry(builder, reg)
swagger.ServeHTTPBridge(app)
```

## 7. 静态资源与 SSE

```go
httpAdapter.ServeStatic("/assets", "./public")

httpAdapter.MountHandler("/feed", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	// 持续写入 event-stream
}))
```

## 8. 共享监听与独立监听

| 模式 | 启动方式 | 适用 |
|------|----------|------|
| 共享监听 | `httpadapter.New()` + `app.Run()` | HTTP + WS/GraphQL 同端口 |
| 独立监听 | `httpadapter.New(":8080")` + `app.Run()` | 单独 HTTP 服务 |

{{< callout type="warning" >}}
同一个应用里最多只能有一个 adapter 要求共享监听地址；否则 `Listen` 会报错。
{{< /callout >}}
