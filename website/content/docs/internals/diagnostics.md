---
title: 诊断能力
weight: 3
prev: /docs/internals/adapter-plugin
next: /docs/best-practices
---

框架提供三层诊断，建议在启动前全部执行一次。

## 1. 编译诊断

```go
diag, err := app.CompileDiagnostics()
if err != nil {
	panic(err)
}
```

能提前发现：

- routeKey 冲突
- handler 签名非法（例如流式方法返回值不正确）
- owner 模块缺失
- binding 声明非法

## 2. 启动报告

```go
if _, err := app.StartupReport(); err != nil {
	panic(err)
}
```

输出示例：

```text
startup report:
  modules=2 adapters=2 protocols=2 routes=9 bindings=46
  registry=instance
  globalAOP middleware=0 guard=0 interceptor=2 pipe=0 filter=0
  adapters:
    - http (shared-listener)
    - ws (shared-listener)
  protocols:
    - http plugin=http declarations=6 routes=6 containers=6
    - ws plugin=ws declarations=9 routes=9 containers=9
  modules:
    - *module.BaseModule imports=1 providers=2 controllers=2 resolvers=0 global=false
```

关注点：

| 字段 | 含义 |
|------|------|
| `registry=instance` | 使用实例级 registry（推荐） |
| `registryFallbacks` | 是否走了全局兼容回退，新代码应避免 |
| `runtimeFallbacks` | 运行期回退项（如 binding→DI） |
| `routes=0` | 声明确实没写进去 |

## 3. 依赖图

```go
gd := app.GraphDiagnostics()
```

用于排查：

- 模块循环依赖
- provider 归属异常
- 某个 controller 为什么没有 owner

## 4. 运行期观测

| 能力 | 入口 |
|------|------|
| Scheduler 快照 | `scheduler.MountHTTPBridge(app, adp, "/scheduler/status")` |
| 健康检查 | `integrations/terminus` |
| 链路追踪 | `integrations/otel` |
| 统一错误元信息 | `Executor.enrichMeta`（自动附带 routeKey/paramIndex/bindingName） |

## 5. 排查顺序建议

```text
1. CompileDiagnostics  → 声明是否合法
2. StartupReport       → 声明是否真的装进 Runtime
3. 单测 Execute        → Runtime 链路是否通
4. 端到端请求          → 传输层是否通
5. 观察 fallbacks      → 是否走了兼容路径
```
