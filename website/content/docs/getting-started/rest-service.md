---
title: 构建一个带鉴权的 REST 服务
weight: 4
prev: /docs/getting-started/project-structure
next: /docs/core-concepts
---

本章把快速开始的示例扩展成一个可用的 REST 服务：

- 模块拆分（Store 模块 + Controller）
- 依赖注入（Controller 自动拿到 Store）
- 请求体校验（`validate` tag）
- 全局鉴权（Guard + Interceptor）
- Swagger 文档

## 1. 拆分模块

```go
type StoreModule struct {
	*module.BaseModule
	store *Store
}

func NewStoreModule(store *Store) *StoreModule {
	provider, _ := di.ProviderFromValue[*Store](store, di.Singleton)
	return &StoreModule{
		BaseModule: module.New(module.Metadata{
			Providers: []*di.Provider{provider},
			Exports:   []any{di.TokenFor[*Store]()}, // 只有导出的 token 才能被导入方注入
		}),
		store: store,
	}
}
```

## 2. Controller 通过字段注入拿到依赖

```go
type UserController struct {
	Store *Store // 由 BaseModule.Init() 自动从容器注入
}

type createUserArgs struct {
	Ctx  httpbinding.Context
	Body httpbinding.Body[createUserRequest]
}

type createUserRequest struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gte=0"`
}

func (c *UserController) Create(args createUserArgs) (any, error) {
	item := c.Store.CreateUser(args.Body.Value)
	args.Ctx.Status(http.StatusCreated)
	return item, nil
}
```

> Controller 以**零值**传入模块即可，`Init` 阶段会走 `Class` 构造管线填充字段。

## 3. 在控制器内声明路由与鉴权

```go
func (c *UserController) Declare(r *httpadapter.Routes) {
	r.Prefix("/users").
		UseGuards(auth.RequireAuthenticated()).
		Get("", c.List).
			Summary("List Users").
			Tags("users").
			Security("bearerAuth").
			ResponseEnvelope(apiEnvelope{}, "data").
		Post("", c.Create).
			Summary("Create User").
			Tags("users").
			Security("bearerAuth").
			Status(http.StatusCreated).
			ErrorResponse(http.StatusBadRequest, "参数错误")
}
```

规则只有三条：`Prefix/Use*` 在第一个动词前作用于整个控制器；动词开始一条新路由；其余修饰作用于当前路由。

## 4. 装配模块树

```go
httpModule := httpadapter.Module(httpadapter.Config{
	Addr:      ":8080",
	Validator: true, // 启用 validate tag 校验
})

root := module.New(module.Metadata{
	Imports: []module.Module{
		httpModule,
		swaggerModule,
		NewStoreModule(store),
	},
	Controllers: []any{&AuthController{}, &UserController{}},
})

app, err := application.New(root, application.Options{Logger: os.Stdout})
if err != nil {
	log.Fatal(err)
}
```

## 5. 全局拦截器

```go
app.UseGlobalInterceptors(aop.InterceptorFunc(func(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
	result, err := next.Handle()
	if err != nil {
		return nil, err
	}
	return apiEnvelope{Code: 200, Msg: "ok", Data: result, Success: true}, nil
}))
```

需要模块级 AOP（等价于 Nest 的 `APP_GUARD`/`APP_INTERCEPTOR`）时，
模块在 `Init` 里声明：

```go
func (m *MyModule) Init() error {
	if err := m.BaseModule.Init(); err != nil {
		return err
	}
	m.Context().Use(module.AOP{Middlewares: []aop.MiddlewareFunc{...}})
	return nil
}
```

## 6. 接入 Swagger

```go
swaggerModule := swagger.Module(
	swagger.Config{
		Title:       "Demo API",
		Version:     "1.0.0",
		BearerAuth:  []string{"bearerAuth"},
		DefaultErrorResponse: "请求失败",
	},
	&swagger.UIConfig{
		SpecURL:    "/docs/openapi.json",
		SwaggerURL: "/docs",
		RedocURL:   "/redoc",
	},
)
```

把 `swaggerModule` 放进根模块的 `Imports` 即可，无需再手工调用
`BuildFromHTTPRegistry` 或 `ServeHTTPBridge`。

启动后：

- `GET /docs` → Swagger UI
- `GET /docs/openapi.json` → OpenAPI 3.0.3 描述
- `GET /redoc` → ReDoc

## 7. 启动

```go
if err := app.Run(); err != nil {
	log.Fatal(err)
}
```

`Run` 内部依次完成：声明（Declare）→ 编译 → 启动适配器 → 阻塞。

## 8. 验证清单

| 场景 | 期望 |
|------|------|
| `POST /auth/login` 参数缺失 | `400` + `kind=Validation` |
| `POST /auth/login` 正常 | `200` + token |
| 未带 token 访问 `/users` | `401` |
| 带 token 查询 | `200` + 列表 |
| `GET /docs` | `200` Swagger UI |

完整可运行版本见 `cmd/http-demo`。
