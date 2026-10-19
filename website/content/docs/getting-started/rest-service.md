---
title: 构建一个带鉴权的 REST 服务
weight: 4
prev: /docs/getting-started/project-structure
next: /docs/core-concepts
---

本章把快速开始的示例扩展成一个可用的 REST 服务：

- 模块拆分（Store 模块 + Controller 模块）
- 依赖注入（Controller 自动拿到 Store）
- 请求体校验（`validate` tag）
- 全局鉴权拦截器
- Swagger 文档

## 1. 拆分模块

```go
var storeToken = reflect.TypeFor[*MemoryStore]()

type StoreModule struct {
	*module.BaseModule
	store *MemoryStore
}

func NewStoreModule() *StoreModule {
	store := NewMemoryStore()
	pStore, _ := di.ProviderFromInstanceWithToken(storeToken, store, di.Singleton)
	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pStore},
		Exports:   []any{storeToken}, // 只有导出的 token 才能被 imports 方注入
	})
	return &StoreModule{BaseModule: base, store: store}
}
```

## 2. Controller 通过字段注入拿到依赖

```go
type UserController struct {
	Store *MemoryStore // 由 BaseModule.Init() 自动从容器注入
}

type createUserArgs struct {
	Ctx  httpbinding.Context
	Body httpbinding.Body[createUserRequest]
}

type createUserRequest struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gt=0"`
}

func (c *UserController) Create(args createUserArgs) (any, error) {
	req := args.Body.Value
	item := c.Store.CreateUser(req.Name, req.Email, req.Age)
	args.Ctx.Status(http.StatusCreated)
	return item, nil
}
```

> Controller 以**零值**传入模块即可，`Init` 阶段会走 `Class` 构造管线填充字段。

## 3. 装配模块树

```go
storeModule := NewStoreModule()
authCtrl := &AuthController{}
userCtrl := &UserController{}

root := module.CreateModule(
	[]module.Module{storeModule}, // imports
	nil,                          // providers
	[]any{authCtrl, userCtrl},    // controllers
	nil, nil,
)

app, err := application.NewWithOptions(root, application.Options{
	Registry:        registry.New(),
	StartupReporter: os.Stdout,
}, httpAdapter)
```

## 4. 声明路由与校验

```go
httpAPI := httpAdapter.API().(*httpadapter.API)

httpAPI.Controller("/users", userCtrl).
	Get("", userCtrl.List).Security("bearerAuth").Build()

httpAPI.Controller("/users", userCtrl).
	Post("", userCtrl.Create).
	Summary("Create User").
	Tags("users").
	Security("bearerAuth").
	ResponseEnvelope(apiEnvelope{}, "data").
	StatusCode(http.StatusCreated).
	Build()
```

```go
httpAdapter := httpadapter.New().WithValidatorV10()
```

`WithValidatorV10()` 会校验 `validate:"..."` 标签，并把错误归一化为 `KernelError{Kind: Validation}`。

## 5. 全局鉴权拦截器

```go
func authInterceptor(store *MemoryStore) aop.InterceptorFunc {
	return func(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
		if isPublicRoute(ctx.RouteKey) {
			return next.Handle()
		}
		hc, _ := ctx.HandlerContext.(*runtime.HandlerContext)
		token, err := extractBearerToken(hc.Request.Headers["Authorization"])
		if err != nil {
			return nil, err
		}
		account, ok := store.FindAccountByToken(token)
		if !ok {
			return nil, aop.UnauthorizedException("登录已失效")
		}
		hc.Set("demo.auth.account", account)
		return next.Handle()
	}
}

app.UseGlobalInterceptors(authInterceptor(store))
```

## 6. 接入 Swagger

```go
swaggerModule, _ := swagger.ForRoot(swagger.Config{
	Title: "Demo API", Version: "1.0.0",
}, &swagger.UIConfig{
	SpecURL: "/docs/openapi.json",
	SwaggerURL: "/docs",
})

builder, _ := module.GetByType[*swagger.Builder](app.ModuleRef())
builder.AddBearerAuth("bearerAuth")
if _, err := swagger.BuildFromHTTPRegistry(builder, reg); err != nil {
	panic(err)
}
swagger.ServeHTTPBridge(app)
```

启动后：

- `GET /docs` → Swagger UI
- `GET /docs/openapi.json` → OpenAPI 3.0.3 描述

## 7. 验证清单

| 场景 | 期望 |
|------|------|
| `POST /auth/register` 弱密码 | `400` + `kind=Validation` |
| `POST /auth/register` 正常 | `201` |
| 未带 token 访问 `/users` | `401` |
| 带 token 分页查询 | `200` + 列表 |
| 访问不存在的用户 | `404` |

完整可运行版本见 `cmd/http-demo`。
