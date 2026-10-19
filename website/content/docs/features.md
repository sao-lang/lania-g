---
title: 特性总览
weight: 2
toc: false
---

这页是**全量特性地图**。每一项都标注了对应的原理与用法章节，可以当作学习清单逐条打勾。

## 1. 内核能力

| 特性 | 说明 | 章节 |
|------|------|------|
| 模块树 | `imports/providers/controllers/resolvers/exports` 五槽位 | [Module](core-concepts/module) |
| 唯一构造入口 | `module.New(Metadata)` / `module.AsGlobal(Metadata)` | [参考：Module & DI](reference/module-di) |
| 带配置模块 | 集成与协议统一 `X.Module(cfg)` | [参考：Module & DI](reference/module-di) |
| 循环引用 | `ForwardRef` 延迟解析 | [参考：Module & DI](reference/module-di) |
| 依赖注入 | 父子容器、三种作用域、字段/构造/Injectable 注入 | [依赖注入](core-concepts/di) |
| 类型化取值 | `di.GetByType` / `module.GetByType` | [依赖注入](core-concepts/di) |
| 声明注册 | 路由、AOP、Binding、配置统一写入 Registry | [编译与执行](core-concepts/compile-runtime) |
| 编译期检查 | 冲突检测、owner 解析、签名校验、AOT 产物 | [编译与执行](core-concepts/compile-runtime) |
| 运行时执行 | Router → Binding → Pipeline → Executor | [请求执行链](internals/request-flow) |
| 上下文池化 | `AcquireHandlerContext` / `ReleaseHandlerContext` | [请求执行链](internals/request-flow) |
| 统一错误模型 | `KernelError` + 协议映射 | [错误与生命周期](core-concepts/errors-lifecycle) |
| 生命周期钩子 | Module / Application 四级钩子 | [错误与生命周期](core-concepts/errors-lifecycle) |
| 热加载 | `Application.HotLoad` | [参考：Application](reference/application) |

## 2. AOP 能力

| 特性 | 说明 | 章节 |
|------|------|------|
| Middleware | 包裹整个执行过程 | [AOP](core-concepts/aop) |
| Guard | 允许/拒绝执行 | [AOP](core-concepts/aop) |
| Interceptor | 包裹 handler 调用 | [AOP](core-concepts/aop) |
| Pipe | 入参/返回值转换 | [AOP](core-concepts/aop) |
| Param Pipe | 按参数索引的 Pipe | [参考：AOP](reference/aop) |
| Exception Filter | 错误消费与转换 | [参考：AOP](reference/aop) |
| 全局 / 控制器 / 路由三级挂载 | `Use*` 链式声明 | [AOP](core-concepts/aop) |
| 编译期 AOP 计划 | `CompiledAOP`，避免重复合并 | [AOP](core-concepts/aop) |

## 3. 协议能力

### HTTP

`Routes` 链（GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS/ALL）、path 参数、query、header、cookie、form、multipart、JSON body、validator v10、CORS、Helmet、静态资源、mounted handler、basePath、body 大小限制、SSE、renderer、redirect、Swagger 注解、声明期错误累积（由 `app.Run()` 报出）。

→ [HTTP 指南](protocols/http) · [HTTP 参考](reference/http)

### gRPC

unary、server streaming、client streaming、bidi streaming、`CompositeStruct`、`Req[T]`、`Header[T]`、metadata、reflection、请求类型推断（含 `Empty` 默认）、返回值桥接（任意 JSON 值 → proto）。

→ [gRPC 指南](protocols/grpc) · [gRPC 参考](reference/grpc)

### GraphQL

Query/Mutation/Object、`Returns`、`Arg`、guard、`RequirePermission`、`RateLimit`、`Timeout`、`CacheControl`、`Deprecation`、playground、复杂度限制、内省开关、嵌套类型递归推导、嵌入字段摊平、自引用保护。

→ [GraphQL 指南](protocols/graphql) · [GraphQL 参考](reference/graphql)

### WebSocket

namespace 网关、事件声明、响应/错误事件后缀、连接/断开/错误钩子、房间、房间广播、namespace 广播、消息绑定、validator、共享监听与独立监听、Socket.IO v4。

→ [WebSocket 指南](protocols/websocket) · [WebSocket 参考](reference/websocket)

### MQ

Consumer/Group/Topic 声明、可插拔 Driver、ack/nack、重试计数、并发消费、消息体/header/topic/key 绑定。

→ [MQ 指南](protocols/mq) · [MQ 参考](reference/mq)

### Scheduler

After/Every/Cron、Retry、MaxConcurrency、Unique、WithTimeout、Misfire 策略、参数绑定、运行快照、指标。

→ [Scheduler 指南](protocols/scheduler) · [Scheduler 参考](reference/scheduler)

## 4. 集成能力（18 个）

auth（JWT/API Key/Session/多租户/身份提供方）、cache（内存/Redis）、config（文件/环境变量/内存合并）、logger（结构化、多输出、轮转）、orm（GORM、多数据源、事务、Repository）、mongodb（Repository 模式）、otel（tracing/metrics/propagation）、outbox（发件箱、重试、死信、调度）、resilience（限流/超时/重试/熔断/幂等）、events（进程内事件总线）、http client（重试、拦截器）、grpc client、kafka、minio、es、swagger、terminus（健康检查）、config。

→ [集成总览](integrations) · [集成参考](reference/integrations)

## 5. 工程能力

| 特性 | 说明 |
|------|------|
| 多模块工作区 | 每个子模块独立 `go.mod`、独立发布 |
| 单一入口 | 每个能力只有一个公开 API，历史 `*Compat` 已移除 |
| 启动诊断 | `app.Diagnostics()` / `app.StartupReport()` / `app.GraphDiagnostics()` |
| 示例端口可配置 | `PORT` / `APP_ADDR` / `GRPC_ADDR` |
| 测试三层 | 单元 / 编译+执行 / 端到端 |
| 文档站 | Hugo + Hextra，搜索与 Mermaid 本地化 |

→ [测试策略](best-practices/testing) · [诊断能力](internals/diagnostics)
