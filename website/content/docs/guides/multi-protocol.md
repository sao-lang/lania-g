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
        shared :18080 │               │ :19051 (standalone)
   ┌─────────────────┴────┐     ┌─────┴──────────┐
   │ HTTP adapter         │     │ gRPC adapter   │
   │  ├─ /hello           │     └────────────────┘
   │  ├─ /graphql         │
   │  └─ /socket.io/      │
   └──────────────────────┘
```

关键规则：**`Addr` 留空 = 挂到 HTTP 上；写了 `Addr` = 自己监听**。

| 协议 | 写 `Addr` | 不写 `Addr` |
|------|-----------|-------------|
| HTTP | 自己监听（必须） | 报错 |
| WebSocket | 自己监听 | 挂到 HTTP 的 `/socket.io/` |
| GraphQL | 自己监听 | 挂到 HTTP 的 `/graphql` |
| gRPC | 自己监听（只能这样） | 不支持共享 |

{{< callout type="info" >}}
一个应用里可以有多个协议适配器；`app.Run()` 会按模块树顺序启动全部 adapter。
{{< /callout >}}

## 2. 装配

多协议 = 多个协议模块放进同一个根模块：

```go
root := module.New(module.Metadata{
	Imports: []module.Module{
		httpadapter.Module(httpadapter.Config{}),            // 共享监听承载者
		wsadapter.Module(wsadapter.Config{}),                // 挂 /socket.io/
		graphqladapter.Module(graphqladapter.Config{Playground: true}), // 挂 /graphql
		grpcadapter.Module(grpcadapter.Config{Addr: addr.GRPC(":19051")}), // 独立
	},
	Controllers: []any{httpCtrl, gateway, userSvc},
	Resolvers:   []any{query},
})

app, err := application.New(root, application.Options{Logger: os.Stdout})
if err != nil { log.Fatal(err) }

// 统一决定共享监听地址（HTTP 承载它）；必须在 Run 之前调用。
if err := app.Listen(addr.HTTP(":18080")); err != nil { log.Fatal(err) }

if err := app.Run(); err != nil { log.Fatal(err) }
```

`cmd/all-in-one` 就是这份代码的可运行版本：

```bash
cd cmd && go run ./all-in-one
# 或用环境变量覆盖
APP_ADDR=:18080 GRPC_ADDR=:19051 go run ./all-in-one
```

{{< callout type="warning" >}}
`app.Listen(addr)` 只作用于实现了 `adapter.SharedListenerConfigurator` 且
`RequiresSharedListen()` 为 true 的 adapter（当前只有 HTTP）。最多允许一个，
多个会直接报错。
{{< /callout >}}

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
cd cmd && go run ./all-in-one
```

| 入口 | 验证方式 |
|------|----------|
| HTTP | `GET :18080/hello` |
| GraphQL | `GET :18080/graphql?query={ping}` 或 Playground `/playground` |
| WebSocket | `socket.io-client@4` 连接 `:18080`，命名空间 `/ws/chat` |
| gRPC | `grpc.NewClient(":19051")` 调用 `allinone.Demo/Ping` |

## 8. 常见坑

| 现象 | 原因 |
|------|------|
| `no mounted adapter requires a shared listener address` | 调了 `app.Listen`，但没引入 HTTP 模块 |
| `multiple adapters require shared listener address` | 有两个 adapter 都想当共享监听者 |
| `http.Module requires http.Config{Addr: ":8080"}` | HTTP 模块既没配 `Addr`，也没调 `app.Listen` |
| WS/GraphQL 没挂到 HTTP 上 | 它们配置了 `Addr`，被当成 standalone 独立监听 |
| WS 连接失败 | 客户端不是 `socket.io-client@4` |
| GraphQL 字段找不到 | `.Returns(...)` 的类型与 Go 模型不一致 |
