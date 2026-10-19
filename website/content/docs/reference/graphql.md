---
title: GraphQL
weight: 8
prev: /docs/reference/grpc
next: /docs/reference/websocket
---

包：`github.com/sao-lang/lania-g/protocol/graphql/v3`

## 1. Adapter

```go
gqlAdp := graphqladapter.New().WithPlayground(true)   // 共享监听
app, _ := application.NewWithOptions(root, application.Options{
	Registry: registry.New(),
}, gqlAdp)
```

| 方法 | 说明 |
|------|------|
| `New(addr...)` | 无参=共享监听；带参=独立监听（默认路径 `/graphql`） |
| `WithPath(path)` | 修改端点路径 |
| `WithAddr(addr)` | 修改监听地址 |
| `AsStandalone(addr)` | 转为独立监听 |
| `WithSchema(*Schema)` | 注入手写 schema |
| `WithPlayground(bool)` / `MountPlayground(path)` | Playground 开关与路径 |
| `WithMaxBodyBytes(n)` | 请求体上限 |
| `WithErrorFormatter(fn)` | 自定义错误序列化 |
| `WithValidator(v)` / `WithValidatorV10()` | 参数校验 |
| `WithContextFactory(fn)` | 自定义 `GraphQLContext` 构造 |
| `API()` / `Handler()` | DSL 上下文 / `http.Handler` |

## 2. DSL

```go
gqlAPI := gqlAdp.API().(*graphqladapter.API)

gqlAPI.UseComplexityLimit(200)

gqlAPI.Resolver("Query", query).
	Query("user", query.UserWithStore).
		Arg("id").Returns("UserResult").
		Description("按 ID 查询用户").
		CacheControl("private, max-age=0").
		UseGuards(graphqlAuthGuard(store)).
	Query("users", query.Users).
		Arg("page").Arg("size").Arg("keyword").
		Returns("UserListResult").
		RateLimit(20, 1000).
	Build()

gqlAPI.Resolver("User", user).
	Object("displayName", user.DisplayName).
	Build()
```

### ResolverBuilder

| 方法 | 作用 |
|------|------|
| `Query/Mutation/Object/Subscription(name, handler)` | 声明字段（`Subscription` 暂不支持编译） |
| `UseMiddlewares/UseGuards/UseInterceptors/UsePipes/UseFilters` | resolver 级 AOP |
| `UseParamPipes(index, ...)` | 参数级 Pipe |
| `Build()/BuildE()/Err()` | 收尾 |

### FieldBuilder

| 方法 | 作用 |
|------|------|
| `Arg(name, opts...)` | 追加参数 |
| `Args(...*ArgBuilder)` | 批量参数 |
| `Returns(typeName)` / `ObjectType` | 声明返回类型名（嵌套选择集分发用） |
| `Description` / `Deprecation` | 文档 |
| `CacheControl` | cache-control 元数据 |
| `RequirePermission(string)` | 权限标记 |
| `Timeout(ms)` | 字段超时元数据 |
| `Complexity(n)` | 复杂度权重 |
| `RateLimit(limit, period)` | 限流元数据 |
| `UseGuards/UseInterceptors/UsePipes/UseFilters` | 字段级 AOP |
| `Build()/BuildE()` | `Build` 会结束整个 resolver 的链式声明 |

### ArgOption

```go
.Arg("id", graphqladapter.WithRequired(true), graphqladapter.WithDescription("用户 ID"))
.Arg("page", graphqladapter.WithDefault(1))
```

## 3. 类型推导

DSL 的 `Arg` 只声明名字（类型由 handler 签名与 schema 推导），**返回类型由 Go 结构自动生成**：

| Go 类型 | GraphQL |
|---------|---------|
| `int` / `int64` | `Int` |
| `float64` | `Float` |
| `string` | `String` |
| `bool` | `Boolean` |
| `time.Time` | `String` |
| `map[string]any` | JSON 标量 |
| `[]T` | `[T]` |
| `*T` | 可空字段 |
| 嵌套 struct | 自动生成 `ObjectType` 并注册 |
| 匿名嵌入字段 | 摊平到父类型 |
| `json:"-"` | 跳过 |
| 自引用结构 | 有递归保护 |

```go
type userView struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Profile struct {
		Bio string `json:"bio"`
	} `json:"profile"`        // 自动注册为嵌套 Object
	Secret  string `json:"-"` // 不进 schema
}
```

## 4. 请求绑定

见 [Binding 矩阵](binding#graphql)。

```go
func (r *QueryResolver) User(args struct {
	ID    gqlbinding.Arg[int]       `arg:"id" required:"true"`
	Trace gqlbinding.Header[string] `header:"x-trace-id"`
}) (*userView, error)
```

| wrapper | 用途 |
|---------|------|
| `Arg[T]` / `ArgValue[T]` | 字段参数 |
| `Parent[T]` | 父节点（`Object` 字段用） |
| `Header[T]` | 请求头（无绑定名 → 零值） |
| `Headers` | 全部请求头 |
| `Context` | `GraphQLContext` |
| `Variables` / `Extensions` | 查询变量与扩展 |
| `SelectionSet` / `Info` / `Root` | 选择集与执行信息 |
| `OperationName` / `FieldName` / `RawQuery` | 查询元信息 |
| `IP/Host/Method/URL/Path/Session` | 请求元信息 |

## 5. 执行上下文

```go
func (r *Resolver) Trace(ctx gqlbinding.Context) (string, error) {
	return ctx.Request().Header.Get("X-Trace-Id"), nil
}
```

`GraphQLContext` 提供 `Request()`、`Set/Get`（同一请求内跨 resolver 共享）、`Context()`、`Schema` 等。

> AOP 写入的扩展数据通过 `HandlerContext` 读取，不要绕过它直接用 `context.WithValue` 传业务数据。

## 6. 复杂度与限流

```go
gqlAPI.UseComplexityLimit(200)         // validation 阶段强制执行
fb.Complexity(5)                        // 字段权重
fb.RateLimit(20, 1000)                  // 声明元数据
fb.Timeout(5000)                        // 声明元数据
fb.RequirePermission("user:create")     // 声明元数据
```

- `ComplexityLimit` 超限直接返回 `graphql complexity %d exceeds limit %d`
- `RateLimit` / `Timeout` / `RequirePermission` 只写入声明，**执行依赖业务侧 guard / interceptor**
- 框架没有内置「查询深度」限制，深查询请用复杂度上限或自定义扩展

## 7. 内省与 Playground

```go
gqlAPI.DisableIntrospection()   // 生产建议关闭
gqlAdp.WithPlayground(false)    // 生产关闭
```

内省被关闭时，`__schema` / `__type` 查询返回 `introspection is disabled`。

## 8. 扩展与错误

```go
gqlAPI.UseExtensions(myTracingExtension{})
```

```go
type Extension interface {
	BeforeOperation(*OperationContext) error
	AfterOperation(*OperationContext, *GraphQLResponse)
}
```

错误处理：

- `KernelError` → `errors[].message` + `extensions.kind`
- `aop.HttpException` 的 `Status` → `extensions.status`
- resolver panic 被 recover 为内部错误
- 自定义序列化：`gqlAdp.WithErrorFormatter(func(err error) any { ... })`
