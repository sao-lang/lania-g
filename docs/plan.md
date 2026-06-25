# v3 演进计划

## 1. 目标

本计划用于指导当前多 module 版本在未来约 6 个月内的演进方向，重点不是推翻现有架构，而是在当前分层模型基础上补齐稳定性、工程化、可观测性与扩展规范。

核心判断如下：

- 当前 `v3` 的总体架构方向是正确的。
- `Application -> Module/DI -> Registry -> Compiler -> Runtime -> Adapter/Binding/Integrations` 的分层边界已经比较清晰。
- 后续重点应转向“收口、增强、规范化”，而不是继续扩大内核设计面。

本计划遵循以下原则：

- `core` 收口，减少继续膨胀。
- 新能力优先落在 `integrations/*`。
- 跨协议横切能力优先走 `AOP + integration bridge`。
- 公共包稳定优先，内部实现允许演进。
- 优先降低全局状态与隐式 panic 带来的不确定性。

### 1.1 状态约定

后续执行过程中，完成一个点就直接在本文档中标注状态，约定如下：

- `[ ]` 未开始
- `[x]` 已完成
- `[-]` 暂不推进或已废弃

如有必要，可在任务后追加简短说明，例如“完成日期”或“关联 PR/提交说明”。

## 2. 当前架构判断

### 2.1 当前优势

- 多协议统一执行模型清晰，协议差异主要由 `adapter/*` 与 `compiler.ProtocolPlugin` 吸收。
- `Registry` 承接声明，`Compiler` 承接编译，`Runtime` 承接执行，职责划分明确。
- `Module + DI + Integrations` 的模型适合后续扩展外部基础设施能力。
- 文档、目录结构与核心抽象基本一致，具备作为稳定基线继续演进的条件。

### 2.2 当前主要问题

- 多协议编译安装存在路由覆盖风险，属于优先级最高的问题。
- 全局 `Registry` 仍然是兼容路径，测试隔离、多应用并存时仍有状态污染风险。
- 多个 adapter plugin 中存在较多重复编译逻辑，后续维护成本偏高。
- 缺少统一的编译诊断输出，排查声明冲突和路由归属问题效率不高。
- `Must...` 与部分 helper 的 panic 路径仍偏多，不利于基础设施稳定性。
- 可观测性、治理能力、启动诊断仍偏弱，不利于生产环境落地。

## 3. 总体路线

整体按照三阶段推进：

- 第一阶段：先修底座，保证稳定性与可预测性。
- 第二阶段：抽象共性，降低维护成本，提升开发体验。
- 第三阶段：在稳定底座上补高价值新能力。

节奏建议：

- `0-2 月`：P0/P1 稳定性问题。
- `2-4 月`：编译模板化、错误模型收敛、启动与诊断增强。
- `4-6 月`：新 integrations 与生产可用能力建设。

阶段补充：

- 阶段性收口记录已统一归档到 `docs/架构收敛文档.md`
- 当前推荐路径与公共边界以 `docs/API.md`、`docs/标准接入模板.md` 为准

## 4. 第一阶段：稳定底座

时间范围：`0-2 月`

目标：

- 修复当前最关键的不确定性问题。
- 补足组合测试和诊断能力。
- 为后续新增能力提供稳定底座。

### 4.1 P0: 修复多协议编译安装覆盖风险

涉及目录：

- `core/compiler`
- `core/runtime`

主要问题：

- 编译产物安装到 `Router` 时，当前实现存在覆盖之前路由表的风险。
- 多协议安装顺序不稳定时，最终行为可能不可预测。

目标动作：

- [x] 将 `Router.InstallCompiledProtocol(...)` 从“整体替换 routes”改为“增量合并 routes”。（2026-04-11）
- [x] 对协议安装顺序做稳定化处理，例如按协议 ID 或协议枚举顺序安装。（2026-04-11）
- [x] 增加重复 `routeKey` 检测与错误提示。（2026-04-11）
- [x] 明确“路由冲突”是编译错误还是安装错误，并统一策略。（2026-04-11，统一为“编译阶段报错，安装阶段防御性兜底”）

交付物：

- [x] 修复后的路由安装逻辑。（2026-04-11）
- [x] 多协议并存回归测试。（2026-04-11）
- [x] 路由冲突诊断输出。（2026-04-11）

### 4.2 P1: 弱化全局 Registry

涉及目录：

- `application`
- `core/registry`
- `adapter/*/dsl.go`
- `binding/*`

目标动作：

- [x] 将实例级 `Registry` 作为标准示例和推荐路径。（2026-04-11，应用侧通过 mounted adapter API 走实例级 registry）
- [x] 保留 `registry.Global()` 作为兼容机制，但不再作为新代码推荐入口。（2026-04-11）
- [x] 梳理哪些 DSL 默认落全局，哪些可以绑定到 app 实例。（2026-04-11，保留全局 DSL，同时通过 `adapter.API()` 暴露实例级入口）
- [x] 补充“多 app 并存”和“测试隔离”的明确示例与测试。（2026-04-11）

交付物：

- [x] 统一的实例级 `Registry` 使用示例。（2026-04-11，见应用侧回归测试）
- [x] 全局/实例两种模式的行为说明。（2026-04-11，见应用侧回归测试）
- [x] 相关回归测试。（2026-04-11）

### 4.3 P1: 增加编译诊断能力

涉及目录：

- `core/compiler`
- `application`

目标动作：

- [x] 增加 compile report / diagnostics。（2026-04-11）
- [x] 在应用启动前能看到声明、编译产物和冲突信息。（2026-04-11，通过 `Application.CompileDiagnostics()`）

建议输出内容：

- [x] 已注册协议插件列表。（2026-04-11）
- [x] 每个协议的声明数量。（2026-04-11）
- [x] 每个协议编译出的路由数量。（2026-04-11）
- [ ] 未归属 module/container 的 handler 列表。
- [x] 冲突 routeKey 列表。（2026-04-11）
- [x] 已注册 binding resolver 数量。（2026-04-11）
- [x] 全局 AOP 汇总信息。（2026-04-11）

交付物：

- [x] `CompileDiagnostics` 或类似结构。（2026-04-11）
- [x] 应用侧查询入口。（2026-04-11）
- [x] 更清晰的报错与日志输出。（2026-04-11）

### 4.4 P1: 补核心组合测试

涉及目录：

- `application`
- `adapter/http`
- `adapter/grpc`
- `adapter/ws`
- `adapter/graphql`
- `core/registry`

测试重点：

- [x] 多协议同时挂载。（2026-04-11）
- [x] 多个 `Application` 同时存在。（2026-04-11）
- [x] 独立 `Registry` 互不污染。（2026-04-11）
- [x] 全局 AOP 与协议级 AOP 组合执行。（2026-04-11）
- [x] 编译失败与冲突场景的诊断可读性。（2026-04-11）

交付物：

- [x] 一组针对架构风险的回归测试，而不是仅验证 happy path。（2026-04-11）

## 5. 第二阶段：降低维护成本

时间范围：`2-4 月`

目标：

- 将 adapter plugin 中的重复逻辑抽出来。
- 收敛错误模型。
- 增强启动体验和开发体验。

### 5.1 P1: 抽取插件编译模板

涉及目录：

- `core/compiler`
- `adapter/http`
- `adapter/grpc`
- `adapter/ws`
- `adapter/graphql`
- `adapter/mq`
- `adapter/scheduler`

当前问题：

- 多个 plugin 在扫描、owner 归属、AOP 转换、route container 建立、handler plan 组装上存在重复逻辑。

目标动作：

- [x] 在 `core/compiler` 增加公共 helper 或模板结构。（2026-04-11，新增 plugin helper，统一 AOP/owner/installer/param plan 编译辅助）
- [x] adapter plugin 只保留协议差异部分。（2026-04-11，新增 compile loop helper，协议侧进一步收敛到声明差异与 route/tree 差异）
- [x] 统一冲突处理、错误提示与编译输出格式。（2026-04-11，进一步收敛到 compiler helper 与统一 installer）

交付物：

- [x] 统一的 plugin helper。（2026-04-11）
- [x] 各协议 plugin 接入模板后的收敛实现。（2026-04-11，grpc/mq/scheduler/http/ws/graphql 已接入统一 helper，scanner owner 解析也已收敛）
- [x] 适配器之间更一致的行为表现。（2026-04-11，grpc/mq/scheduler/http/ws/graphql 编译路径已部分模板化）

### 5.2 P1: 收敛错误模型

涉及目录：

- `adapter/*`
- `binding/*`
- `integrations/*`
- `core/module`
- `core/di`

目标动作：

- [x] 继续推进“非 `Must...` API 走 `error`”的策略。（2026-04-11，补充 `BuildE` / `Load` / `Require` 等显式错误入口）
- [x] 对 builder、factory、helper 的错误路径做梳理。（2026-04-11，统一 `adapter/*` builder 推荐走 `BuildE()`，并收敛 helper 语义）
- [x] 明确哪些 panic 是兼容保留，哪些应该迁移为 error。（2026-04-11，已形成错误模型约定文档）

交付物：

- [x] 一份错误模型约定文档。（2026-04-11，新增 `docs/错误模型约定.md`）
- [x] 更稳定的 builder / helper API 行为。（2026-04-11，builder 显式错误出口与 helper 语义已统一一轮）
- [x] 兼容旧接口但明确新接口推荐用法。（2026-04-11，保留 `Build()` / `Must...` 兼容入口，同时文档推荐 `BuildE()` / `error` 路径）

### 5.3 P2: 启动体验与开发体验增强

涉及目录：

- `application`
- `docs`

目标动作：

- [x] 提供 startup report。（2026-04-11，新增 `Application.StartupReport()`）
- [x] 启动时输出已加载模块、已挂载 adapter、协议数量、路由数量、全局 AOP、关键 integrations 摘要。（2026-04-11，通过 `Options.StartupReporter` 启用）
- [x] 文档中补充“推荐用法”和“排错路径”。（2026-04-11，补充协议绑定与集成使用指南）

交付物：

- [x] 可选的启动报告输出。（2026-04-11）
- [x] 面向业务方的标准接入模板。（2026-04-11，新增 `docs/标准接入模板.md`）
- [x] 更直接的排障指引。（2026-04-11）

### 5.4 P2: Runtime/Binding 性能优化

涉及目录：

- `core/runtime`
- `binding/*`

目标动作：

- [x] 识别反射和 resolver 匹配热点。（2026-04-11，补充 runtime benchmark 并确认执行期 handler 元信息读取热点）
- [x] 引入参数 plan 缓存、binding resolver 匹配缓存。（2026-04-11，统一使用编译期 `ParamPlans` 与 `BindingRegistry` 匹配缓存，并复用当前 handler 元信息避免重复匹配）
- [x] 形成基准测试，避免优化靠感觉推进。（2026-04-11）

交付物：

- [x] benchmark 基线。（2026-04-11）
- [x] 若干热点优化。（2026-04-11，执行期错误补元信息避免重复 `router.Match()`）
- [x] 不破坏公开 API 的性能提升。（2026-04-11）

## 6. 第三阶段：补高价值能力

时间范围：`4-6 月`

目标：

- 在稳定底座之上，优先建设最有生产价值的新功能。
- 新能力尽可能通过 `integrations/*` 承载，而不是继续挤进 `core/*`。

### 6.1 优先方向一：可观测性

建议新增目录：

- `integrations/otel`

目标能力：

- [x] tracing（2026-04-11，`integrations/otel` 提供全局 interceptor span 启动与错误记录）
- [x] metrics（2026-04-11，`integrations/otel` 提供请求计数器）
- [x] context propagation（2026-04-11，支持 `traceparent` 提取与 `context.Context` 贯通）
- [x] trace id / request id 贯通（2026-04-11）
- [x] 统一接入 logger 字段（2026-04-11，提供 logger field bridge）

设计建议：

- [x] 协议无关部分走 integration。（2026-04-11，新增 `integrations/otel`）
- [x] 执行链接入点优先使用 middleware / interceptor / filter。（2026-04-11，当前采用 middleware + interceptor）
- [x] 各 adapter 只负责协议上下文桥接。（2026-04-11，OTel/Auth/Resilience 均通过 integration + AOP 接入，adapter 无需重复实现核心能力）

### 6.2 优先方向二：认证与权限

建议新增目录：

- `integrations/auth`

目标能力：

- [x] JWT（2026-04-11，`integrations/auth` 已支持 Bearer JWT）
- [x] Session（2026-04-11，`integrations/auth` 已支持 Session token）
- [x] API Key（2026-04-11，`integrations/auth` 已支持 API Key）
- [x] RBAC（2026-04-11，提供 `RequireRoles(...)`）
- [x] Tenant Context（2026-04-11，提供租户上下文与注入）

设计建议：

- [x] 统一身份上下文模型。（2026-04-11）
- [x] 通过 `Guard + Binding` 注入用户信息。（2026-04-11）
- [x] 尽量做到 HTTP / gRPC / WS / GraphQL 复用同一套授权语义。（2026-04-11，走统一 `HandlerContext` 头部/metadata 读取）

### 6.3 优先方向三：治理能力

建议新增目录：

- `integrations/rate_limit`
- 或 `integrations/resilience`

目标能力：

- [x] 限流（2026-04-11，`integrations/resilience`）
- [x] 超时（2026-04-11，`integrations/resilience`）
- [x] 重试（2026-04-11，`integrations/resilience`）
- [x] 熔断（2026-04-11，`integrations/resilience`）
- [x] 幂等（2026-04-11，`integrations/resilience`）

设计建议：

- [x] 不要仅服务 HTTP。（2026-04-11）
- [x] 优先沉淀为跨协议通用中间能力。（2026-04-11）
- [x] 使用 AOP 接入，而不是散落在各 adapter 中重复实现。（2026-04-11）

### 6.4 优先方向四：数据层扩展

建议新增目录：

- `integrations/mongodb`

设计建议：

- [x] 作为独立 integration 存在。（2026-04-11，新增 `integrations/mongodb`）
- [x] 继续遵循 `ForRoot / Factory / InjectXxx / XxxRef[T]` 模式。（2026-04-11）
- [x] 不将其硬塞入现有 `integrations/orm`。（2026-04-11）

### 6.5 优先方向五：异步一致性

建议新增目录：

- `integrations/outbox`
- 或增强 `integrations/events` / `integrations/kafka`

目标能力：

- [x] outbox/inbox（2026-04-11，新增 `integrations/outbox`）
- [x] 事务事件发布（2026-04-11，提供 `PublishInTransaction(...)`）
- [x] 重试与死信桥接（2026-04-11）

设计建议：

- [x] 与 `events + mq + orm` 形成组合能力。（2026-04-11，支持 event dispatcher、自定义 dispatcher、`orm.TransactionManager` 事务发布）
- [x] 优先解决业务系统中的一致性落地问题，而不是上来做重量级工作流引擎。（2026-04-11）

## 7. 按目录的演进建议

### 7.1 `application`

建议方向：

- 增加启动报告与编译诊断查询入口。
- 更清晰地区分 `Run()` 与 `Listen()` 的行为和错误提示。
- 加强实例级 `Registry` 的默认接入姿势。

### 7.2 `core/compiler`

建议方向：

- 固定协议安装顺序。
- 提供 compile diagnostics/report。
- 抽象 plugin 编译模板，减少协议实现重复代码。

### 7.3 `core/runtime`

建议方向：

- 修复路由安装覆盖问题。
- 增加路由冲突检测。
- 优化 binding plan 和执行期开销。

### 7.4 `core/registry`

建议方向：

- 保留全局 registry 兼容能力，但继续推动实例化使用。
- 视需要增加快照与诊断辅助能力。

### 7.5 `adapter/*`

建议方向：

- 统一 DSL 错误处理策略。
- 减少 plugin 之间重复实现。
- 新增协议时坚持“声明 + plugin + transport”的既有模型，不改 runtime 主模型。

### 7.6 `binding/*`

建议方向：

- 完善高频参数类型。
- 统一错误提示。
- 补 resolver 相关测试与性能基线。

### 7.7 `integrations/*`

建议方向：

- 作为后续功能增长的主承载层。
- 优先建设观测、安全、治理、数据与一致性能力。
- 统一接入模式，避免每个 integration 风格不同。

## 8. 不建议当前做的事

- 不建议重写 `Runtime`。
- 不建议继续扩大 `public-api` 面。
- 不建议把 `scheduler` 扩成重量级工作流平台。
- 不建议将 `orm` 演化为数据库统一抽象层。
- 不建议在底座问题未修完前快速增加大量新协议。

## 9. 建议里程碑

### M1

- [x] 修复多协议安装风险。（2026-04-11）
- [x] Registry 隔离路径明确。（2026-04-11）
- [x] 组合测试补齐。（2026-04-11）
- [x] 编译诊断初版可用。（2026-04-11）

### M2

- [x] plugin 模板化完成一轮收敛。（2026-04-11）
- [x] 错误模型清晰。（2026-04-11）
- [x] startup report 可用。（2026-04-11）
- [x] binding/runtime 具备基准测试。（2026-04-11）

### M3

- [x] 至少落地 1 到 2 个高价值 integration。（2026-04-11，已落地 `integrations/otel`、`integrations/auth`、`integrations/resilience`、`integrations/mongodb`、`integrations/outbox`）
- [x] 可观测性或认证能力形成标准接入模式。（2026-04-11，OTel 与 Auth 已形成 module + AOP + binding + docs 的标准接入）
- [x] 文档、示例、排障路径明显改善。（2026-04-11，已补标准接入模板及各 integration 接入文档）

## 10. 一句话结论

`v3` 当前最优演进方向不是继续重构架构，而是“稳住 core、规范扩展点、做强 integrations、补齐可观测与治理能力”。如果按这个路线推进，未来新增能力会更容易落地，框架也会更适合长期维护。
