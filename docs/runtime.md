# lania-g Runtime

本文档描述 `kernel/v3/runtime` 的实际实现与协作方式。该包属于内部基础设施层，业务代码通常不应直接依赖它，而应通过 `application/v3` + `protocol/*/v3` + `protocol/*/v3/binding` 完成接入与扩展。

## 1. Runtime 的定位

Runtime 是协议无关的统一执行引擎，其目标是让 HTTP/WS/gRPC/GraphQL/MQ/Scheduler 等协议在运行期共享同一套执行模型：

- 统一的 `HandlerContext`（协议请求上下文抽象）
- 统一的 `Router`（路由匹配）
- 统一的 `BindingRegistry`（参数绑定）
- 统一的 `Pipeline`（AOP 执行链）
- 统一的 `Executor`（每次请求的入口）

协议差异由各 `protocol/*/v3` 模块的 transport 与 `kernel/v3/compiler.ProtocolPlugin` 在“映射与编译”阶段吸收。

## 2. 核心执行链路

```text
client request
  -> adapter transport
  -> runtime.HandlerContext
  -> runtime.Router.Match
  -> runtime.Executor（容器选择 + 参数绑定 + pipeline）
  -> invoke handler
  -> adapter write response
```

## 3. 关键组件

### 3.1 HandlerContext（统一上下文）

`HandlerContext` 是 runtime 的统一请求上下文，核心字段包括：

- `Protocol`：协议标识（例如 `http`、`grpc`、`ws`）
- `RouteKey`：标准形式为 `{protocol}:{method}:{path}`
- `Request/Response`：统一抽象，适配器负责把协议原生对象映射到这里
- `Container`：request-scope DI 容器（通常由 Executor 创建 child container）
- `Metadata`：跨组件透传（例如当前 handler、模块标识、链路信息等）

实现见：[context.go](file:///Users/bytedance/Desktop/files/self/lania-zip/lania-g/kernel/v3/runtime/context.go)。

### 3.2 Router（routeKey 精确匹配 + 协议 matcher）

Router 的匹配顺序是：

1. 精确匹配：`{protocol}:{method}:{path}` 直接命中 routes map
2. 协议 matcher：协议侧可注册 `RouteMatcher` 实现非精确匹配（例如 HTTP 模板路由）

实现见：[router.go](file:///Users/bytedance/Desktop/files/self/lania-zip/lania-g/kernel/v3/runtime/router.go)。

### 3.3 Executor（每次请求的统一入口）

Executor 负责把一次请求拆解为可执行步骤：

- 匹配 handler（Router.Match）
- 选择该 route 对应模块容器（由 compiler 产出的 `RouteContainers` 提供）
- 创建 request-scope child container
- 调用 `BindingRegistry` 解析 handler 入参
- 执行 `Pipeline`（AOP）
- 归一化错误与返回值

注意：route 对应的容器归属与参数计划来自编译期产物，而不是运行时“临时推断”。

### 3.4 Pipeline（AOP 执行链）

Pipeline 对外表现为一组 AOP 能力的统一承载，语义顺序建议按如下理解：

1. Middleware
2. Guard
3. Pipe（输入侧）
4. Interceptor
5. Handler
6. Pipe（输出侧）
7. Exception Filter

全局 AOP 通常由 `registry` 收集，编译期会把全局 AOP 合并进各 handler 的执行计划；同时 runtime 仍保留全局 AOP 的 fallback 入口，用于兼容少量未完全走编译期计划的路径。

当前仍保留的 runtime fallback 主要有：

- `runtime_global_aop`：当某个 handler 没有编译期 `CompiledAOP` 时，`runtime.Pipeline` 仍会应用 runtime 级全局 AOP
- `binding_to_di`：当协议 binding 没有命中参数时，`runtime.Executor` 仍会回退到 DI 容器解析
- `route_to_root_container`：当编译产物没有为某条 route 提供专属容器时，`runtime.Executor` 会回退到 root container

这些 fallback 目前仍用于兼容，但已经会在 `CompileDiagnostics` / `StartupReport` 中显式暴露，便于后续逐步迁移到更纯的编译期执行模型。

## 3.5 Registry Fallback Inventory

除 runtime 执行链上的 fallback 外，当前还保留一组与 `registry.Global()` 相关的 fallback inventory，也会在 `CompileDiagnostics` / `StartupReport` 中以 `registryFallbacks` 输出：

- `compat`
  - `application_global_compat`：`application.New(...)` / `application.NewCompat(...)` 仍会从 `registry.Global()` 构建应用；新代码应优先使用 `application.NewWithOptions(..., Options{Registry: application.NewRegistry()})`
  - `compile_registry_global_compat`：`compiler.CompileCompat(...)` 仍会从 `registry.Global()` 读取声明；新代码应优先使用 `compiler.Compile(moduleRef, reg, ...)`
- `read-fallback`
  - `graphql_schema_global_read`：`adapter/graphql.buildCompiledSchemaCompat(...)` 会从 `registry.Global()` 读取 schema/config/resolver 声明
  - `events_attach_global_read`：`integrations/events.AttachRegisteredHandlersCompat(...)` 会从 `registry.Global()` 读取事件处理器声明
  - `swagger_http_registry_global_read`：`integrations/swagger.BuildFromHTTPRegistryCompat(...)` 会从 `registry.Global()` 读取 HTTP 路由声明

当前语义已经进一步收紧：

- 普通入口必须显式传入实例级 `registry`
- 只有显式 `Compat` helper 才会走 `registry.Global()` 的全局读取路径

这些入口和前面已经显式记录来源的 `write fallback` 不同：它们主要影响“读取哪份声明”，而不是把声明写入全局单例。因此当前治理重点已经从“盘点并命名”转向“把普通 API 的隐式 nil 回退移除，只保留显式 compat helper”。

## 4. 编译期产物如何安装到 Runtime

在 v3 中，路由与 matcher 通常不由 runtime 侧“直接注册”，而是由编译期产物安装：

1. adapter DSL 把声明写入 `registry`
2. `compiler.Compile(...)` 将声明编译为 `CompiledApp`
3. `CompiledApp.Install(rt)` 创建新的 Router，并为每个协议安装 matcher/routes，同时向 Executor 注入：

- `BindingRegistry`
- `RouteContainers`

实现见：[compile.go](file:///Users/bytedance/Desktop/files/self/lania-zip/lania-g/kernel/v3/compiler/compile.go)。

若业务或框架内部明确需要沿用“从全局 registry 读取声明”的兼容语义，可使用 `compiler.CompileCompat(...)` 让该意图更直观。

## 5. 性能与对象生命周期（维护者关注）

Runtime 提供 `HandlerContext` 对象池用于降低高频请求下的分配成本：

- `AcquireHandlerContext(protocol)` 获取
- `ReleaseHandlerContext(hc)` 归还

适配器在高 QPS 场景中可选择使用对象池，但必须严格遵循 reset 约定，避免跨请求串数据。

## 6. 文件结构

```text
core/runtime/
  context.go        HandlerContext / Request / Response
  router.go         Router / RouteMatcher / routeKey
  executor.go       统一执行入口：match -> bind -> pipeline -> invoke
  pipeline.go       AOP 执行链
  binding.go        BindingRegistry 与 BindingResolver
  handler.go        Handler 表示与元信息
  runtime.go        Runtime 入口与装配
  errors.go         错误定义
```
