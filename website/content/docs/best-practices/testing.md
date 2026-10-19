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

## 2. 编译 + 执行测试（推荐主线）

这类测试覆盖「声明 → 编译 → install → Execute」全链路，不需要网络：

```go
func TestUsersRoute(t *testing.T) {
	ctrl := &UserController{}
	p, _ := di.ProviderFromInstance(ctrl, di.Singleton)
	root := module.CreateModule(nil, []*di.Provider{p}, []any{ctrl}, nil, nil)
	if err := root.Init(); err != nil { t.Fatal(err) }
	ref := module.NewModuleRef(root)

	reg := registry.New()
	NewAPI(reg).Controller("/users", ctrl).Get("", ctrl.List).Build()

	compiled, err := compiler.Compile(ref, reg, NewPlugin())
	if err != nil { t.Fatal(err) }

	rt := runtime.NewRuntime()
	if err := compiled.Install(rt); err != nil { t.Fatal(err) }

	ctx := runtime.NewHandlerContext(protocol.Protocol)
	ctx.RouteKey = runtime.BuildRouteKey(protocol.Protocol, http.MethodGet, "/users")
	if _, err := rt.Execute(ctx); err != nil { t.Fatal(err) }
}
```

优点：不依赖端口、不依赖外部服务，失败点清晰。

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
# 单模块
cd protocol/http/v3 && go test ./...

# 全模块
$mods = Get-ChildItem -Path application,cmd,integrations,kernel,protocol -Recurse -Filter go.mod |
  ForEach-Object { $_.Directory.FullName }
foreach ($m in $mods) { Push-Location $m; go build ./...; go test ./...; Pop-Location }
```

## 5. 测试小贴士

| 场景 | 建议 |
|------|------|
| 测试端口冲突 | 用 `httptest` 或环境变量端口 |
| 全局 registry 污染 | 每例 `registry.ResetGlobal()`（`defer`） |
| 需要真实 DB | 标记 `-short` 跳过，或用容器 |
| 断言错误 | 用 `errors.Is(err, kerrors.ErrBindingNotFound)` |
| 时序类测试（scheduler） | 用通道 + 超时，不要 `sleep` 后直接断言 |
