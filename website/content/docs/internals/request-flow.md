---
title: 请求执行链
weight: 1
prev: /docs/integrations
next: /docs/internals/adapter-plugin
---

本章按源码顺序拆解一次请求的完整路径。

## 1. 启动期

```text
application.New(root, Options{})
  └─ ModuleLoader.LoadMultiple(root)
       ├─ 递归 imports（深度优先）
       ├─ 每个模块创建 Container 并 Init()
       ├─ attachContext（Context.Bind / Context.Use）
       └─ 构建 ModuleRef 树
  └─ 发现适配器：实现 ModuleAdapter 的模块贡献 adapter
       └─ adapter.Mount(host)          # 绑定 Registry / mount 能力
  └─ app.Run()
       ├─ prepare：遍历模块调用 Declare(*Routes)（先 ClearDecl）
       ├─ Mount：集成模块挂载原生 handler（swagger / terminus）
       ├─ Compile
       │    ├─ plugin.Scan(moduleRef, reg)
       │    ├─ 冲突检测 / owner 解析
       │    └─ CompiledApp
       ├─ compiled.Install(rt)
       │    ├─ Router 注册 routeKey → Handler
       │    ├─ BindingRegistry 注册 resolver
       │    └─ RouteContainers 绑定模块容器
       └─ adapter.Start()
```

## 2. 请求期

以 HTTP 为例：

```text
http.Server
  └─ Adapter.ServeHTTP
       ├─ helmet / cors
       ├─ basePath + mounted handler 匹配
       ├─ buildHandlerContext
       │    ├─ AcquireHandlerContext(http)
       │    ├─ 读取 body（受 maxBodyBytes 限制）
       │    ├─ 解析 query / headers / form / files
       │    └─ 注入 validator / renderer
       ├─ executeMiddlewareChain
       └─ runtime.Execute(ctx)
```

## 3. `runtime.Execute`

```go
handler, _ := rt.GetRouter().Get(ctx.RouteKey)   // 1. 路由匹配
args, err := executor.ResolveArgs(ctx, handler)  // 2. 参数解析
result, err := pipeline.Run(ctx, handler, args)  // 3. AOP + 调用
```

### 3.1 参数解析

```text
for each paramType:
  binding.Resolve(ctx, handler, paramType, i)
    ├─ findForProtocol(paramType, ctx.Protocol)   # 类型 + 协议双重匹配
    ├─ 命中 → resolver.Resolve
    └─ 未命中 → DI container.Get(paramType)
```

{{< callout type="info" >}}
`findForProtocol` 是 v3 的关键修复：缓存 key 是 `(type, protocol)`，
因此 HTTP 与 GraphQL 的 `AutoStruct` 不会互相抢占。
{{< /callout >}}

### 3.2 容器选择

```text
route.Container（编译期确定的模块容器）
  → 若缺失则回退 root container
```

每次请求再派生出 request child container，用于 `di.Request` 作用域。

## 4. AOP 执行

```text
Middleware → Guard → Interceptor → Pipe(input)
  → handler.Invoke
→ Pipe(output) → Interceptor → Guard → Middleware
```

异常由最外层过滤器处理；panic 会被 recover 并包装为 `ErrExecutionFailed`。

## 5. 响应编码

| 协议 | 编码 |
|------|------|
| HTTP | `writeHTTPResponse` + renderer / envelope |
| gRPC | `grpcvalue.ToMessage` → proto |
| GraphQL | 选择集投影 |
| WS | `conn.Emit(event+suffix, result)` |
| MQ | Ack / Nack |
| Scheduler | 记录运行结果 |

## 6. 关键文件索引

| 关注点 | 文件 |
|--------|------|
| 应用装配 | `application/v3/application.go` |
| 启动/停止 | `application/v3/runtime.go` |
| 模块加载 | `kernel/v3/module/loader.go` |
| 依赖注入 | `kernel/v3/di/container.go` |
| 声明存储 | `kernel/v3/registry/registry.go` |
| 编译 | `kernel/v3/compiler/compile.go` |
| 参数绑定 | `kernel/v3/runtime/binding.go` |
| 执行器 | `kernel/v3/runtime/executor.go` |
| 管道 | `kernel/v3/runtime/pipeline.go` |
