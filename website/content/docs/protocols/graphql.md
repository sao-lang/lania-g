---
title: GraphQL
weight: 3
prev: /docs/protocols/grpc
next: /docs/protocols/websocket
---

包：`github.com/sao-lang/lania-g/protocol/graphql/v3`

## 0. 装配 Config

```go
graphqladapter.Module(graphqladapter.Config{
	Addr:           ":8082",
	Path:           "/graphql",   // 端点路径
	Playground:     true,         // 挂载 GraphQL Playground
	PlaygroundPath: "/playground",
	MaxBodyBytes:   4 << 20,
	Validator:      true,
})
```

## 1. 声明 schema

```go
func (q *QueryResolver) Declare(r *graphqladapter.Routes) {
	r.Resolver("Query").
		Query("users", q.Users).
		Arg("page").Arg("size").Arg("keyword").
		Returns("UserListResult").
		RateLimit(20, 1000)
}

func (m *MutationResolver) Declare(r *graphqladapter.Routes) {
	r.Resolver("Mutation").
		Mutation("login", m.Login).
		Arg("username").Arg("password").
		Returns("LoginResult")
}
```

## 2. 返回类型与嵌套选择集

`.Returns("UserListResult")` 会自动绑定 `UserListResult` 这个 Go 类型，并按字段推导 schema：

```go
type BaseResponse struct {
	Success bool   `json:"success"`
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
}

type UserListResult struct {
	BaseResponse          // 匿名嵌入会被摊平
	List    []User `json:"list"`
	Total   int    `json:"total"`
}
```

客户端可以直接查询：

```graphql
{
  users(page: 1, size: 10) {
    code
    msg
    total
    list { id name email }
  }
}
```

推导规则：

| 规则 | 说明 |
|------|------|
| 嵌套 struct | 递归注册为 GraphQL object |
| 匿名嵌入 | 摊平到父类型（`BaseResponse` → `code/msg/success`） |
| `json:"-"` | 不出现在 schema |
| `map[string]any` | 按 JSON 标量处理 |
| 自引用 | 不会无限递归 |
| 字段名 | 优先 `json` tag，否则首字母小写 |

## 3. 参数绑定

```go
func (r *QueryResolver) Users(args struct {
	Page    int    `json:"page"`
	Size    int    `json:"size"`
	Keyword string `json:"keyword"`
}, hc *runtime.HandlerContext, store *MemoryStore) (UserListResult, error) {
	...
}
```

也可以用显式 wrapper：

```go
func (r *QueryResolver) User(id gqlbinding.Arg[int]) (UserResult, error)
```

## 4. Header 读取

```go
func (r *QueryResolver) Users(args struct{...}, hc *runtime.HandlerContext) (...) {
	traceID := hc.Request.Headers["X-Trace-Id"]
}
```

{{< callout type="info" >}}
`Header[T]` 只有通过 CompositeStruct 上的 `header:"X-Trace-Id"` 指定名称时才按名取值；
直接作为参数且没有名称时，标量类型会返回零值（不会让整个查询失败）。
{{< /callout >}}

## 5. Guard 与权限

```go
func graphqlAuthGuard(store *MemoryStore) aop.GuardFunc {
	return func(ctx *aop.ExecutionContext) (bool, error) {
		if isPublicGraphQLRoute(ctx.RouteKey) {
			return true, nil
		}
		hc, _ := ctx.HandlerContext.(*runtime.HandlerContext)
		if hc == nil || hc.Request == nil {
			return false, aop.InternalServerErrorException("request context unavailable")
		}
		token := hc.Request.Headers["Authorization"]
		if _, ok := store.FindAccountByToken(token); !ok {
			return false, aop.UnauthorizedException("请在 Authorization 头中携带 token")
		}
		return true, nil
	}
}
```

{{< callout type="warning" >}}
Guard 的签名是 `func(ctx *aop.ExecutionContext) (bool, error)`，**不是** interceptor 风格；返回 `(false, nil)` 会被归一化为 `KindForbidden`。
{{< /callout >}}

未通过时 GraphQL 响应：

```json
{ "errors": [{ "message": "请在 Authorization 头中携带 token", "path": ["users"] }] }
```

## 6. Playground

```go
gqlAdapter := graphqladapter.New().WithPlayground(true)
```

- `GET /playground` → GraphQL Playground
- `POST /playground` → 查询端点（默认路径由 adapter 决定）

## 7. 常见错误

| 错误 | 原因 |
|------|------|
| `graphql field X.Y not found` | 查询字段不存在于 schema |
| `graphql type X not found` | 返回类型没有绑定 Go 模型，或嵌套类型未注册 |
| `binding not supported for protocol: protocol=graphql` | 多协议下命中错误 binding（已在 v3 修复） |
| `json: cannot unmarshal object into Go value of type string` | `Header[string]` 缺少 binding name |
