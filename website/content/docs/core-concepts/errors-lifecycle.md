---
title: 错误与生命周期
weight: 6
prev: /docs/core-concepts/binding
next: /docs/protocols
---

## 1. 错误模型

框架统一使用 `kernel/v3/errors.KernelError`：

```go
type KernelError struct {
	Kind    Kind
	Message string
	Cause   error
	Meta    map[string]any
}
```

常见 `Kind`：

| Kind | 语义 | HTTP 状态 |
|------|------|-----------|
| `KindValidation` | 参数校验失败 | 400 |
| `KindBinding` | 参数绑定失败 | 400 |
| `KindUnauthorized` | 未认证 | 401 |
| `KindForbidden` | 无权限 | 403 |
| `KindRouteNotFound` | 路由不存在 | 404 |
| `KindDI` | 依赖解析失败 | 400/500 |
| `KindExecutionFailed` | handler panic 被恢复 | 500 |

## 2. 抛错方式

```go
return nil, aop.BadRequestException("username 不能为空")
return nil, aop.UnauthorizedException("登录已失效")
return nil, aop.NotFoundException("用户不存在")
return nil, aop.ConflictException("账号已存在")
return nil, aop.ForbiddenException("no permission")
```

也可以直接构造：

```go
return nil, &kerrors.KernelError{
	Kind:    kerrors.KindValidation,
	Message: "invalid email",
	Meta:    map[string]any{"field": "email"},
}
```

## 3. 协议如何消费错误

| 协议 | 行为 |
|------|------|
| HTTP | `writeHTTPError` 映射状态码 + JSON body |
| gRPC | 转为 gRPC status（`Unknown`/`Unauthenticated` 等） |
| GraphQL | 写入 `errors` 数组 |
| WS | emit `event + ErrorSuffix` |
| MQ | 执行 `Nack(err)` |
| Scheduler | 记录失败并按 `Retry` 配置重试 |

## 4. 异常过滤器

```go
func errorFilter(ctx *aop.ExecutionContext, err error) (any, error) {
	if errors.Is(err, myapp.ErrNotFound) {
		return apiEnvelope{Code: 404, Msg: "not found"}, nil
	}
	return nil, err
}
```

返回 `nil` 表示错误已被消费，上层不再继续抛出。

## 5. 生命周期

```text
application.New(root, Options)
  → 模块加载 / 容器构建（含 Init 中的注册与绑定）
  → OnModuleInit
  → Declare（遍历模块，收集各协议声明）
  → Mount（集成模块挂载 handler，如 swagger / terminus）
  → Compile / Install
  → OnApplicationBootstrap
  → adapter.Start（开始监听）
  → [运行中]
  → app.Stop()
  → adapter.Stop（逆序）
  → OnModuleDestroy / OnApplicationShutdown
```

## 6. 优雅关闭

```go
if err := app.Run(); err != nil {
	panic(err)
}
```

`Run()` 会阻塞，直到收到退出信号或：

```go
if err := app.Stop(); err != nil {
	log.Println(err)
}
```

`Stop()` 会按 adapter 挂载的逆序停止服务，再执行 shutdown 钩子。

## 7. 启动前自检建议

```go
if err := app.Run(); err != nil {
	panic(err) // 声明冲突、owner 缺失、签名非法
}
```

启动报告（`app.StartupReport()`）会输出模块数、路由数、binding 数，
非常适合排查"为什么路由没生效"。
