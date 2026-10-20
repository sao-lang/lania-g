---
title: 测试策略
weight: 1
prev: /docs/internals/diagnostics
next: /docs/best-practices/troubleshooting
---

`lania-g` 的测试分三层，成本与覆盖范围递增。

## 1. 单元测试（最快）

只测业务逻辑，不启动框架：

```go
func TestUserService_Create(t *testing.T) {
	svc := NewUserService(fakeStore{})
	user, err := svc.Create("Tom", "tom@example.com")
	if err != nil { t.Fatal(err) }
	if user.ID == 0 { t.Fatal("id not assigned") }
}
```

## 2. 声明 + 编译测试（推荐主线）

只验证「控制器声明是否合法、路由是否装进 Runtime」：把 `app.Run()` 的 `error` 当断言。
声明冲突、owner 缺失、签名非法都会在这一步暴露。

```go
func TestRouteDeclaration(t *testing.T) {
	root := module.New(module.Metadata{
		Imports:     []module.Module{httpadapter.Module(httpadapter.Config{Addr: ":0", Validator: true})},
		Controllers: []any{NewUserController(fakeStore{})},
	})

	app, err := application.New(root, application.Options{})
	if err != nil {
		t.Fatal(err)
	}

	report, err := app.StartupReport()
	if err != nil {
		t.Fatal(err)
	}
	if report.RouteCount == 0 {
		t.Fatal("expected routes declared")
	}
}
```

{{< callout type="info" >}}
`StartupReport()` 返回模块数 / 适配器数 / 路由数 / binding 数的结构化快照，
拿它做断言比解析日志稳定。
{{< /callout >}}

协议内部的参数解析、AOP 计划生成属于包内细节。为了让源码目录保持干净，
**所有测试都放在仓库根目录的 `test/` 模块**，通过 dot-import 访问被测包：

```go
package http_test

import . "github.com/sao-lang/lania-g/protocol/http/v3"
```

{{< callout type="info" >}}
`test/` 是一个独立的 Go module（已加入 `go.work`），按领域再分目录：
`test/kernel`、`test/application`、`test/protocol`、`test/integrations`、`test/e2e`。
需要构造声明的场景使用各协议导出的 `NewRoutes(reg, instance)`，无需把测试塞回源码目录。
{{< /callout >}}

## 3. 端到端测试（最后一道）

启动真实 adapter，用真实客户端验证协议行为。

### HTTP

```go
srv := httptest.NewServer(adapter)
defer srv.Close()
resp, _ := http.Get(srv.URL + "/users/ping")
```

### WebSocket

```js
const socket = io('http://127.0.0.1:18081/ws/chat', { transports: ['polling'] });
socket.emit('ping', {});
socket.on('pingResponse', (data) => console.log(data));
```

### gRPC

```go
conn, _ := grpc.NewClient("127.0.0.1:19051", grpc.WithTransportCredentials(insecure.NewCredentials()))
resp := &structpb.Struct{}
err := conn.Invoke(ctx, "/AuthService/Register", req, resp)
```

### MQ / Scheduler

- MQ：注入内存 driver，推一条消息，断言 handler 命中与 ack
- Scheduler：注册 `Every(100ms)` 任务，等待若干次执行后断言计数与快照

## 4. 回归命令

```bash
# 全量测试（含协议、集成、端到端）
cd test && go test ./...

# 单个领域
cd test && go test ./protocol/http/...
```

源码目录不再包含 `*_test.go`；`go build ./...` 在每个模块里只验证编译。

## 5. 测试小贴士

| 场景 | 建议 |
|------|------|
| 测试端口冲突 | 用 `httptest` 或环境变量端口 |
| 全局 registry 污染 | 每例 `registry.ResetGlobal()`（`defer`） |
| 需要真实 DB | 标记 `-short` 跳过，或用容器 |
| 断言错误 | 用 `errors.Is(err, kerrors.ErrBindingNotFound)` |
| 时序类测试（scheduler） | 用通道 + 超时，不要 `sleep` 后直接断言 |
