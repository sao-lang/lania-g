---
title: 多协议同进程
weight: 2
prev: /docs/guides/full-flow-http
next: /docs/guides/deployment
---

HTTP、WebSocket、GraphQL 可以共享同一个监听端口；gRPC 单独监听。

## 1. 拓扑

```text
                ┌──────────────────────────────┐
                │  Application                 │
                │  ├─ ModuleLoader / DI        │
                │  ├─ Compile / Install        │
                │  └─ Adapters                 │
                └──────────────────────────────┘
                     │                │
        shared :8080 │                │ :50051 (standalone)
   ┌─────────────────┴────┐     ┌─────┴──────────┐
   │ HTTP adapter         │     │ gRPC adapter   │
   │  ├─ /users           │     └────────────────┘
   │  ├─ /graphql         │
   │  └─ /socket.io/      │
   └──────────────────────┘
```

- HTTP 用 `httpadapter.New()`（共享监听）
- GraphQL 用 `graphqladapter.New()`（共享监听，自动挂到 HTTP）
- 每个协议用 `X.Module(X.Config{Addr: ...})` 声明自己的监听地址
- `app.Run()` 会依次启动全部适配器

{{< callout type="info" >}}
一个应用里可以有多个协议适配器；它们各自监听自己的地址，互不冲突。
{{< /callout >}}

## 2. 装配

多协议 = 多个协议模块放进同一个根模块：

```go
root := module.New(module.Metadata{
	Imports: []module.Module{
		httpadapter.Module(httpadapter.Config{Addr: ":8080", Validator: true}),
		graphqladapter.Module(graphqladapter.Config{Addr: ":8082"}),
		wsadapter.Module(wsadapter.Config{Addr: ":8081"}),
		grpcadapter.Module(grpcadapter.Config{Addr: addr.GRPC(":50051")}),
	},
	Controllers: []any{userCtrl, gateway, userSvc},
	Resolvers:   []any{query},
})

app, err := application.New(root, application.Options{Logger: os.Stdout})
```

每个协议模块声明自己的监听地址，`app.Run()` 统一启动全部适配器。

## 3. 声明

每个协议都由控制器自己的 `Declare` 方法声明，互不干扰：

```go
// HTTP
func (c *UserController) Declare(r *httpadapter.Routes) {
	r.Prefix("/users").Get("", c.List)
}

// GraphQL（resolver 放在 Resolvers 槽位）
func (q *QueryResolver) Declare(r *graphqladapter.Routes) {
	r.Resolver("Query").Query("users", q.Users).Arg("page").Arg("size").Returns("UserListResult")
}

// WebSocket
func (g *ChatGateway) Declare(r *wsadapter.Routes) {
	r.Gateway("/ws/chat").On("users_list", g.UsersList)
}

// gRPC
func (s *UserService) Declare(r *grpcadapter.Routes) {
	r.Service("UserService").Method("List", s.List)
}
```

{{< callout type="info" >}}
同一个 struct 可以同时实现多个协议的 `Declare` 方法（例如既暴露 HTTP 又暴露 gRPC），
因为它会被多个模块的 `Controllers` 槽位引用；声明阶段按协议分别收集。
{{< /callout >}}

## 4. 跨协议共享 DI

同一个 handler 依赖（`*Store`）可以被四种协议同时注入：

```go
type UserController struct{ Store *Store }
type QueryResolver  struct{ Store *Store }
type ChatGateway    struct{}
type UserService    struct{ Store *Store }
```

前提：`*Store` 已由某个模块 provider 注册并 `Exports`。

## 5. 跨协议共享 AOP

```go
app.UseGlobalInterceptors(traceInterceptor)
app.UseGlobalGuards(rateLimitGuard)
```

全局 AOP 会作用于所有协议的 handler。

## 6. 协议感知的 Binding

多协议共存时，参数解析会同时校验**类型**与**协议**：

```text
findForProtocol(paramType, ctx.Protocol)
  ├─ 类型匹配 + 协议允许 → 使用该 resolver
  ├─ 类型匹配但协议不允许 → 跳过，继续找下一个
  └─ 都没命中 → 回退 DI
```

因此 HTTP 的 `AutoStruct` 不会抢走 GraphQL 的 `AutoStruct`。

## 7. 启动与验证

```bash
PORT=18080 GRPC_ADDR=:19051 go run ./cmd/all-in-one
```

| 入口 | 验证方式 |
|------|----------|
| HTTP | `GET /users/ping` |
| GraphQL | `POST /graphql` |
| WebSocket | `socket.io-client@4` 连接 `/ws/chat` |
| gRPC | `grpc.NewClient` 调用 unary / stream |

## 8. 常见坑

| 现象 | 原因 |
|------|------|
| `shared listener adapters present; use app.Run()` | 调用了 `app.Run()`，但存在共享监听 adapter |
| `multiple adapters require shared listener address` | 有两个 adapter 都想当 listener |
| WS 连接失败 | 客户端不是 `socket.io-client@4` |
| GraphQL 字段找不到 | `.Returns(...)` 的类型与 Go 模型不一致 |
