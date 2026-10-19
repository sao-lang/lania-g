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
- WebSocket 用 `wsadapter.New()`（共享监听，挂到 `/socket.io/`）
- gRPC 用 `grpcadapter.New(":50051")`（独立监听）

{{< callout type="warning" >}}
一个应用里**只能有一个** adapter 要求共享监听地址。HTTP 是唯一提供 listener 的角色，
GraphQL / WS 只是挂载 handler，因此组合是安全的。
{{< /callout >}}

## 2. 装配

```go
httpAdapter := httpadapter.New().WithValidatorV10()
gqlAdapter := graphqladapter.New().WithPlayground(true)
wsAdapter := wsadapter.New()
grpcAdapter := grpcadapter.New(addr.GRPC(":50051"))

app, err := application.NewWithOptions(root, application.Options{
	Registry: application.NewRegistry(),
}, httpAdapter, gqlAdapter, wsAdapter, grpcAdapter)
```

## 3. 声明

### HTTP

```go
httpAPI := httpAdapter.API().(*httpadapter.API)
httpAPI.Controller("/users", userCtrl).Get("", userCtrl.List).BuildE()
```

### GraphQL

```go
gqlAPI := gqlAdapter.API().(*graphqladapter.API)
gqlAPI.Resolver("Query", query).
	Query("users", query.Users).Arg("page").Arg("size").Returns("UserListResult").BuildE()
```

### WebSocket

```go
wsAPI := wsAdapter.API().(*wsadapter.API)
wsAPI.Gateway("/ws/chat", gateway).
	On("users_list", gateway.UsersList).
	BuildE()
```

### gRPC

```go
grpcAPI := grpcAdapter.API().(*grpcadapter.API)
grpcAPI.Service("UserService", userSvc).
	Method("List", userSvc.List).
	BuildE()
```

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
| `shared listener adapters present; use app.Listen(addr)` | 调用了 `app.Run()`，但存在共享监听 adapter |
| `multiple adapters require shared listener address` | 有两个 adapter 都想当 listener |
| WS 连接失败 | 客户端不是 `socket.io-client@4` |
| GraphQL 字段找不到 | `.Returns(...)` 的类型与 Go 模型不一致 |
