---
title: AOP
weight: 4
prev: /docs/core-concepts/compile-runtime
next: /docs/core-concepts/binding
---

`lania-g` 提供五类增强，执行顺序固定，语义各不相同。

## 1. 执行顺序

```text
Middleware
  → Guard
    → Interceptor
      → Pipe（入参）
        → Handler
      ← Pipe（返回值）
    ← Interceptor
  ← Guard
← Middleware
```

异常过滤（`ExceptionFilter`）在最外层捕获错误。

## 2. 五类增强对比

| 类型 | 能否改变返回值 | 能否中断 | 典型用途 |
|------|----------------|----------|----------|
| `Middleware` | 可以 | 可以 | 日志、耗时统计、CORS |
| `Guard` | 不能 | 可以（拒绝） | 鉴权、权限校验 |
| `Interceptor` | 可以 | 可以 | 缓存、重试、统一响应包裹 |
| `Pipe` | 可以（Transform） | 可以（报错） | 参数转换、脱敏 |
| `ExceptionFilter` | 可以（消费错误） | — | 错误归一化、上报 |

## 3. 声明方式

### 全局

```go
app.UseGlobalInterceptors(responseEnvelopeInterceptor, authInterceptor(store))
app.UseGlobalGuards(permGuard)
app.UseGlobalPipes(trimPipe)
app.UseGlobalFilters(errorFilter)
```

### 模块级

模块的 `Metadata.AOP` 或 `Init()` 里的 `Context.Use` 都会并入全局 AOP 链：

```go
root := module.New(module.Metadata{
	Imports: []module.Module{auth.Module(auth.Config{...})},
	AOP: module.AOP{
		Guards:       []aop.GuardFunc{permGuard},
		Interceptors: []aop.InterceptorFunc{traceInterceptor},
	},
})
```

集成包（`auth` / `resilience` / `otel`）在模块 `Init()` 内部调用 `Context.Use(...)`，
所以引入模块即生效，无需手工注册。

### 模块级（按路由范围）

`Metadata.AOP` 是"全应用生效"，如果需要**只对部分路由生效**，让模块实现
`Configure(consumer aop.MiddlewareConsumer)`：

```go
type AccessScopeModule struct{ *module.BaseModule }

func (m *AccessScopeModule) Configure(consumer aop.MiddlewareConsumer) {
	consumer.Apply(auditMiddleware("users")).
		ForRoutes("/users/*").
		Exclude("GET /users")

	consumer.Apply(auditMiddleware("admin")).
		ForRoutes("GET /admin/*")
}
```

把它放进根模块的 `Imports` 即可。规则支持前缀 `*`、单段参数 `:id`、方法前缀 `GET /path`，
详见 [AOP 参考](../reference/aop)。

### 控制器 / 路由级

控制器在 `Declare` 中声明，`Prefix`/`Use*` 在第一条路由之前是**控制器作用域**，
之后的 `Use*` 只作用于当前路由：

```go
func (c *UserController) Declare(r *httpadapter.Routes) {
	r.Prefix("/users").
		UseGuards(authGuard).          // 控制器级
		UseInterceptors(traceInterceptor)

	r.Get("", c.List)
	r.Get("/:id", c.Get).UseGuards(adminGuard) // 只作用于这一条
}
```

## 4. 写一个 Interceptor

```go
func responseEnvelopeInterceptor(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
	result, err := next.Handle()
	hc, _ := ctx.HandlerContext.(*runtime.HandlerContext)

	if err != nil {
		status, msg := statusAndMessageFromError(err)
		if hc != nil {
			hc.Response.Status = status
		}
		return apiEnvelope{Msg: msg, Code: status, Success: false}, nil
	}
	return apiEnvelope{Msg: "ok", Data: result, Success: true}, nil
}
```

{{< callout type="warning" >}}
Interceptor 返回 `nil, nil` 会**吞掉响应**：HTTP 不会写任何内容。
需要吞掉错误时请显式返回一个响应对象。
{{< /callout >}}

## 5. 写一个 Guard

```go
func permGuard(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
	if !hasPermission(ctx) {
		return nil, aop.ForbiddenException("no permission")
	}
	return next.Handle()
}
```

## 6. 编译期 AOP

如果声明阶段已经确定了 AOP 列表，编译产物会携带 `CompiledAOP`。
此时 Runtime **不再**把全局 AOP 重新合并进去，避免重复执行。

{{< callout type="info" >}}
排查"某个中间件执行了两次"时，先看 `handler.Meta.CompiledAOP` 是否非空。
{{< /callout >}}
