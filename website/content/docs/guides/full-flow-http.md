---
title: HTTP 完整流程
weight: 1
prev: /docs/guides
next: /docs/guides/multi-protocol
---

本章用一个带鉴权的用户服务，把框架的每个阶段串起来：**你在每一步都能看到“发生了什么、为什么需要它”**。

## 0. 目录规划

```text
demo/
├── main.go
├── module/
│   ├── store.go        # StoreModule
│   └── root.go         # RootModule
├── controller/
│   ├── auth.go
│   └── user.go
├── dto/
│   └── user.go
└── interceptor/
    ├── auth.go
    └── envelope.go
```

## 1. 定义 DTO 与业务存储

```go
// dto/user.go
type CreateUserRequest struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gt=0"`
}
```

```go
// module/store.go
type Store struct {
	mu     sync.Mutex
	users  []User
	tokens map[string]Account
}

func NewStore() *Store { /* ... */ }
```

## 2. StoreModule：注册并导出 provider

```go
var storeToken = reflect.TypeFor[*Store]()

type StoreModule struct {
	*module.BaseModule
	store *Store
}

func NewStoreModule() *StoreModule {
	store := NewStore()
	p, _ := di.ProviderFromInstanceWithToken(storeToken, store, di.Singleton)
	return &StoreModule{
		BaseModule: module.NewBaseModule(&module.ModuleMetadata{
			Providers: []*di.Provider{p},
			Exports:   []any{storeToken},
		}),
		store: store,
	}
}

func (m *StoreModule) OnModuleInit() error    { return m.store.Warmup() }
func (m *StoreModule) OnModuleDestroy() error { return nil }
```

**为什么**：`Exports` 决定父模块能否注入 `*Store`；没有它，Controller 会得到 `provider not found`。

## 3. Controller：零值 + 字段注入

```go
type UserController struct {
	Store *Store // 由 Init 阶段自动注入
}

type listUsersArgs struct {
	Ctx     httpbinding.Context
	Page    httpbinding.Query[int]    `query:"page"`
	Size    httpbinding.Query[int]    `query:"size"`
	Keyword httpbinding.Query[string] `query:"keyword"`
}

func (c *UserController) List(args listUsersArgs) (any, error) {
	items, total := c.Store.List(args.Keyword.Value, args.Page.Value, args.Size.Value)
	return map[string]any{"list": items, "total": total}, nil
}

type createUserArgs struct {
	Ctx  httpbinding.Context
	Body httpbinding.Body[CreateUserRequest]
}

func (c *UserController) Create(args createUserArgs) (any, error) {
	req := args.Body.Value
	user := c.Store.Create(req.Name, req.Email, req.Age)
	args.Ctx.Status(http.StatusCreated)
	return user, nil
}
```

**为什么**：Controller 不需要构造函数，也不用手动传依赖；容器在模块 `Init` 时填充 exported 字段。

## 4. 根模块：组合

```go
// module/root.go
func NewRootModule(authCtrl, userCtrl any) module.Module {
	return module.CreateModule(
		[]module.Module{NewStoreModule()}, // imports
		nil,                               // providers
		[]any{authCtrl, userCtrl},         // controllers
		nil,                               // resolvers
		nil,                               // exports
	)
}
```

## 5. 装配应用

```go
func main() {
	storeModule := NewStoreModule()
	authCtrl := &AuthController{}
	userCtrl := &UserController{}
	root := module.CreateModule(
		[]module.Module{storeModule}, nil,
		[]any{authCtrl, userCtrl}, nil, nil,
	)

	httpAdapter := httpadapter.New().WithValidatorV10()

	app, err := application.New(root, application.Options{Logger: os.Stdout}, httpAdapter)
	if err != nil {
		panic(err)
	}
	...
}
```

**为什么**：`NewWithOptions` 会完成三件事：加载模块树、构建容器、挂载 adapter。
传入实例级 Registry 可以避免全局状态污染。

## 6. 声明路由

```go
httpAPI := httpAdapter.API().(*httpadapter.API)

httpAPI.Controller("", authCtrl).
	Post("/auth/register", authCtrl.Register).StatusCode(201).Build().
	Post("/auth/login", authCtrl.Login).Build()

httpAPI.Controller("/users", userCtrl).
	Get("", userCtrl.List).Security("bearerAuth").
	Summary("List Users").Tags("users").Build().
	Post("", userCtrl.Create).StatusCode(201).Security("bearerAuth").Build().
	Get("/:id", userCtrl.Get).Security("bearerAuth").Build()
```

**为什么**：DSL 只写声明，不执行任何反射；真正的 handler 在编译期生成。

## 7. 全局 AOP

```go
app.UseGlobalInterceptors(responseEnvelopeInterceptor, authInterceptor(store))
```

- `responseEnvelopeInterceptor`：统一响应包裹与错误折叠
- `authInterceptor`：读取 `Authorization`，校验 token，写入请求上下文

## 8. 编译 + 安装 + 启动

```go
if _, err := app.CompileDiagnostics(); err != nil {
	panic(err) // 声明冲突 / owner 缺失 / 签名非法
}
if _, err := app.StartupReport(); err != nil {
	panic(err)
}
if err := app.Run()); err != nil {
	panic(err)
}
```

启动报告里应看到：

```text
routes=7 bindings=35
protocols:
  - http plugin=http declarations=7 routes=7 containers=7
```

如果 `routes=0`，说明声明没有写进**当前应用的 Registry**。

## 9. 验证

```bash
# 注册
curl -X POST http://127.0.0.1:8080/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"123456"}'

# 登录
TOKEN=$(curl -s -X POST http://127.0.0.1:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"demo","password":"123456"}' | jq -r .data.token)

# 带鉴权查询
curl http://127.0.0.1:8080/users?page=1\&size=10 \
  -H "Authorization: Bearer $TOKEN"
```

## 10. 每一步的失败点

| 阶段 | 典型失败 | 排查 |
|------|----------|------|
| 模块加载 | `provider not found` | 检查 `Exports` |
| Controller 注入 | 字段仍为零值 | 检查字段是否 exported、是否为零值 |
| DSL 声明 | `BuildE` 报错 | 检查路径/方法名/重复声明 |
| 编译 | `module owner not found` | controller 是否放进 `controllers` |
| 启动 | `address already in use` | 用 `PORT` 覆盖 |
| 请求 | `401/404` | 检查 routeKey、鉴权拦截器、basePath |

## 11. 下一步

- 想接入 WebSocket / GraphQL 到同一端口 → [多协议同进程](multi-protocol)
- 想查每个 API 的完整签名 → [参考手册](../reference/http)
