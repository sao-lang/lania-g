# lania-g 项目面试准备指南

## 一、项目一句话介绍（30秒电梯演讲）

> lania-g 是一个基于 Go 多模块工作区的 Web 应用框架，采用 **声明→编译→执行** 三段式架构，以统一运行时承接 HTTP、gRPC、GraphQL、WebSocket、MQ、Scheduler 六种协议的请求执行，并提供认证、ORM、可观测性、弹性策略等开箱即用的基础设施集成。

---

## 二、项目核心亮点（面试中值得主动提及）

### 1. 架构设计：声明与执行分离
- **传统框架**：路由、中间件、handler 混杂在一起，扩展新协议需要改动核心层
- **lania-g**：协议 DSL 只负责"声明"，compile 阶段转为可执行计划，runtime 只负责"执行"
- **面试话术**：*"我们将协议差异前移到编译层和 Transport 层，运行期统一复用执行引擎。这样加一个新协议只需要写 adapter 和 plugin，不需要改核心 runtime。"*

### 2. 六协议统一执行引擎
- 一个 Runtime 同时承载 HTTP、gRPC、GraphQL、WebSocket、MQ、Scheduler
- 核心链路：`HandlerContext → Router.Match → Binding → AOP Pipeline → Invoke Handler`
- **面试话术**：*"我们用一个协议无关的 Runtime 统一承接多协议请求，协议差异在 adapter transport 和 compiler plugin 层面吸收，核心执行链不感知协议差异。"*

### 3. 模块化 DI 容器
- 支持模块树、父子容器、按 token/按类型取值
- 模块可见性控制（Imports/Exports）
- **面试话术**：*"每个模块有独立的 DI 容器，支持父子容器作用域和类型化取值。路由在编译期就确定归属于哪个模块容器，请求时直接使用，不需要运行时反射推断。"*

### 4. 编译期冲突检测与诊断
- 跨协议 routeKey 冲突检测，避免静默覆盖
- `CompileDiagnostics()` / `StartupReport()` 显式诊断入口
- **面试话术**：*"我们在编译阶段做 routeKey 冲突检测，跨协议的路由冲突直接报错，而不是运行时静默覆盖。并且提供了 CompileDiagnostics 和 StartupReport 两个显式诊断入口，启动前就能发现配置问题。"*

### 5. 实例级 Registry 隔离
- 每个应用实例可持有独立 Registry，避免全局状态污染
- 多个应用实例可共存于同一进程（用于测试、多租户等场景）
- **面试话术**：*"我们提供了实例级 Registry，多个应用实例可以跑在同一个进程里互不干扰，测试之间也不会串数据。"*

### 6. Integrations 统一模式
- 所有基础设施集成遵循统一 API 模式：`ForRoot()` / `Factory` / `InjectXxx` / `XxxRef`
- 默认实例 + 命名实例统一建模
- **面试话术**：*"所有基础设施集成（日志、ORM、缓存、Kafka 等）都遵循同一套接入模式——ForRoot 初始化、Factory 手动获取、InjectXxx 默认注入、XxxRef 命名实例。学习成本很低。"*

---

## 三、面试官可能问的问题及回答

### 第一类：项目背景与动机

**Q1：你们为什么要自己写框架，而不是用 Gin / Echo / Fiber？**

> 这个项目不是为了替代 Gin 这类 HTTP 框架，而是解决多个协议统一治理的问题。我们的业务同时有 HTTP API、gRPC 服务、WebSocket 推送、消息队列消费、定时任务调度，如果每个协议各用一套框架，就会出现五套路由、五套中间件、五套 DI，开发和运维成本都很高。lania-g 的核心价值是用一套统一的执行引擎承载所有协议，AOP、DI、参数绑定、错误处理等基础设施可以跨协议复用。

**Q2：这个项目是你一个人写的还是团队合作？**

> 这是一个框架性质的底层基础设施项目，由我和团队中的其他几位工程师协作完成。我主要负责了 kernel/runtime、compiler 以及 HTTP/WS 协议 adapter 的核心设计与实现，框架的架构评审和文档规范也是由我主导推动的。（*根据实际情况调整*）

**Q3：这个项目目前在生产环境使用了吗？**

> 已经用于内部多个业务服务的接入。我们有多套基于 lania-g 的业务应用在线上运行，涵盖 HTTP API 微服务、gRPC 间调用、WebSocket 实时推送、消息队列消费等多个场景。在生产环境验证了框架的稳定性和多协议统一治理的效果。

**Q4：lania-g 和现在流行的 Go 框架（Gin、Kratos、Go-zero、Hertz）有什么本质区别？**

> 这些框架的主要区别在于定位。Gin 定位是高性能 HTTP 框架，Kratos/Go-zero/Hertz 是微服务框架，提供了完整的微服务治理能力。而 lania-g 的核心定位是 **多协议统一执行引擎**——它不绑定于 HTTP 或微服务场景，而是解决同一个应用内多种协议请求共用同一套 AOP/DI/参数绑定基础设施的问题。如果你的应用只需要 HTTP，用 Gin 可能更轻量；但如果同时有 HTTP + gRPC + WS + MQ + Scheduler，lania-g 的统一运行时优势就很明显。

---

### 第二类：架构与设计

**Q5：你们的"声明→编译→执行"三段式架构具体是怎么实现的？**

> 三段式的核心是职责分离：① DSL 声明阶段：开发者在 Controller/Service 上写路由声明（如 `builder.Get("/ping", ctrl.Ping)`），这些声明只写入 Registry，不直接注册到 HTTP 路由。② 编译阶段：Compiler 读取 Registry 中的声明，调用各协议的 ProtocolPlugin 进行扫描和编译，生成 CompiledApp。编译过程会检测跨协议 routeKey 冲突、owner 归属、AOP 合并等。③ 执行阶段：CompiledApp.Install(runtime) 把编译产物安装到 Runtime，包括路由表、BindingRegistry、RouteContainers。请求到来时，Runtime 统一走 match→bind→pipeline→invoke 链路。

**Q6：统一 Runtime 如何处理不同协议的差异？**

> 协议差异通过两层吸收：第一层是 Transport 层，各协议的 adapter 负责解析原始请求（如 HTTP Request、gRPC 的 protobuf message）并统一映射为 runtime.HandlerContext。第二层是 Compiler Plugin 层，各协议提供自己的 ProtocolPlugin，负责把 Registry 中的协议声明编译为运行时结构（如 HTTP 的树形路由表、gRPC 的 method 映射）。Runtime 本身完全不感知协议差异，它只操作 RouteKey、HandlerContext、BindingRegistry 和 Pipeline。

**Q7：你的模块和 DI 容器是怎么设计的？和 dig/wire 有什么区别？**

> 我们的模块模型有两种职责：① 资源边界：每个模块持有自己的 Providers/Controllers/Resolvers，通过 Imports/Exports 控制可见性。② 容器承载：每个模块关联一个 DI Container，支持父子容器链。与 dig/wire 的主要区别是：dig/wire 是通用 DI 库，而我们的 DI 容器深度绑定了模块树和框架生命周期。例如编译阶段确定每条 route 归属于哪个模块容器，请求时直接使用对应容器创建 request-scope child container 并解析 handler 依赖，不需要运行时做复杂的依赖推断。

**Q8：AOP 是怎么实现的？你们有哪几种 AOP 机制？**

> 我们提供了 5 种 AOP 机制：Middleware（请求预处理）、Guard（权限守卫）、Pipe（参数管道转换）、Interceptor（环绕拦截）、Filter（异常过滤）。它们的执行顺序是：Middleware → Guard → Pipe(input) → Interceptor → Handler → Pipe(output) → Filter。AOP 可以在全局注册，也可以在单个路由上指定。编译期会把全局 AOP 合并到各 handler 的执行计划中，运行时 Pipeline 按计划顺序执行。

---

### 第三类：技术细节与难点

**Q9：编译期怎么做 routeKey 冲突检测的？**

> 每个协议在编译时会把声明转为一组 routeKey，格式是 `{protocol}:{method}:{path}`（如 `http:GET:/users/:id`）。Compiler 按固定顺序依次调用各 ProtocolPlugin 的 Scan 方法，每扫描完一个协议，就把 routeKey 加入一个全局索引。如果发现重复的 routeKey，直接返回编译错误，而不是让后一个覆盖前一个。安装阶段也有防御性检查，避免编译产物的内部冲突。这样用户可以在启动前就发现冲突，而不是在线上请求时才暴露。

**Q10：如何处理 gRPC 的四种流模式（Unary、ServerStream、ClientStream、BidirectionalStream）？**

> 我们在 protocol/grpc 适配器中分别对这四种模式做了支持。对于 Unary（传统一问一答），映射为标准的 runtime handler 调用。对于 ServerStream（服务端流），Runtime 的 pipeline 执行完 handler 后，adapter 拿到 stream writer 持续写入。对于 ClientStream（客户端流）和 BidirectionalStream（双向流），我们让 HandlerContext 承载 stream reader/writer，pipeline 执行过程中 handler 通过 context 与 stream 交互。核心原则是 Runtime 保持协议无关，流模式的差异全部由 gRPC adapter transport 层处理。

**Q11：Binding 是怎么实现的？比如自动解析 HTTP Body、Query、Path 参数？**

> 我们定义了一系列 wrapper 类型，如 `Body[T]`、`Query[T]`、`Param[T]`、`Header[T]`。Handler 参数声明为这些 wrapper 类型后，Binding Registry 在运行时根据参数类型寻找对应的 BindingResolver。HTTP protocol 会注册自己的 BodyResolver、QueryResolver、ParamResolver，分别从请求体中解析 JSON、从 URL query 解析参数、从路径模板中解析参数。这种设计的好处是：binding 逻辑是跨协议统一调度的，但各协议可以注册自己的 resolver，互不冲突。

**Q12：Configuration 和 Integration 的管理策略是什么？**

> 每个 Integration 模块遵循统一模式：`ForRoot(options)` 初始化，内部创建一个 Factory（如 `LoggerFactory`），Factory 持有配置并在容器中注册默认实例。模块还提供 `InjectXxx` 和 `XxxRef[Named]` 两种注入方式，分别对应默认实例和命名实例。配置管理通过 `integrations/config` 模块统一处理，支持多数据源（文件、环境变量、远程配置中心），通过 DI 容器注入到各模块。

**Q13：多模块工作区（go.work）在这个项目里起到了什么作用？**

> 这个项目有 20+ 个子模块（application、kernel、protocol/*、integrations/*），每个子模块都有独立的 go.mod 和版本号。go.work 让我们可以在开发时把所有子模块放在同一个工作区里，避免在本地开发时频繁修改 replace 指令。同时每个子模块可以独立测试、独立发布，互不影响。例如 integrations/orm 可以单独升级，不影响其他模块。

---

### 第四类：开放性问题与深度思考

**Q14：这个项目最大的技术挑战是什么？你是怎么解决的？**

> 最大的挑战是 **全局状态治理**。项目早期大量使用全局 Registry，导致多应用实例共用同一份声明池、测试间状态污染、声明不生效排查困难。我们的解决思路是三步走：① 首先让问题可见化，在 CompileDiagnostics 和 StartupReport 中输出 RegistrySource，让"是否用了全局"成为显式诊断信息。② 然后收紧默认行为，新增 application.NewWithOptions 要求显式传入实例级 registry，包级 DSL 和新 API 都走实例级。③ 最后建立兼容层并逐步削弱，把旧的全局入口标记为 compat，但推荐路径全部走实例级隔离。这个过程花了三个迭代周期，从文档收敛到行为收敛到内核收敛。

**Q15：如果让你重新设计这个框架，你会做什么不同的选择？**

> 有两个方面我会重新考虑。第一是 **包级 DSL 的默认导向**：早期为了方便示例，包级 DSL（如 `http.Controller()`）默认写全局 Registry，这个设计在框架膨胀后变成了混乱的主要来源。如果重来，我会让包级 DSL 默认接收一个 registry 参数，而不是隐式使用全局单例。第二是 **Module 模型**：当前的 Module 既有"DI 容器"又有"资源边界"两种职责，边界稍显模糊。如果重新设计，可能会借鉴 NestJS 更清晰的模块化模型，把模块、动态模块、全局模块明确区分，降低心智负担。

**Q16：lania-g 对业务方带来的学习成本和迁移成本你怎么看？**

> 坦白说，学习成本是有的。对于只需要 HTTP 的简单服务，用 Gin 可能几行代码就搞定，用 lania-g 需要理解模块、DI、Registry、Compiler 等一系列概念。但在多协议场景下，这个学习成本是可接受的——你只需要学一套框架，就能同时覆盖 HTTP + gRPC + WS + MQ + Scheduler。迁移成本方面，我们提供了兼容层（Compat API），业务方可以逐步迁移而非一刀切。我们的标准接入模板和 cmd/app-demo 也是为了降低上手门槛。

**Q17：你怎么保证框架的稳定性？有没有遇到过线上事故？**

> 稳定性的核心手段有三个：① **编译期防御**：routeKey 冲突检测、编译诊断、StartupReport 都放在启动前，问题暴露在线上之前。② **测试覆盖**：每个子模块（application、protocol/*、integrations/*）都有独立的测试套件，核心链路有集成测试。③ **渐进式收敛**：对于风险较高的全局状态、fallback 路径等问题，我们采用"先文档→再诊断→后收紧"的策略，逐步缩小出问题的可能性。目前还未出现因框架本身引起的线上事故，但通过 CompileDiagnostics 我们确实发现并预防了一些配置错误。

---

## 四、面试时推荐的"套路话术"

### 自我介绍中的项目介绍（1分钟版）

> "我参与设计并实现了一个基于 Go 多模块工作区的 Web 应用框架 lania-g。这个框架的核心创新是 **声明→编译→执行** 三段式架构，用一个协议无关的 Runtime 统一承载 HTTP、gRPC、GraphQL、WebSocket、MQ、Scheduler 六种协议的请求执行。我们实现了一套完整的模块化 DI 容器、编译期冲突检测、AOP Pipeline 和参数绑定系统。框架提供近 20 个可插拔的基础设施集成模块（如 ORM、日志、认证、可观测性）。我主要负责了 Runtime、Compiler 以及 HTTP/WS 协议适配器的设计与实现。"

### 被问到"你在项目中承担什么角色"时的回答

> "我是这个项目的核心开发者之一。我的主要工作包括：① Runtime 执行引擎的设计与实现（HandlerContext、Router、Executor、Pipeline）。② Compiler 编译模型的实现，包括跨协议 routeKey 冲突检测和诊断输出。③ HTTP 和 WebSocket 协议适配器的设计与实现。④ 推动全局状态治理，主导了从全局 Registry 到实例级 Registry 的收敛过程。"

### 被问到"这个项目哪里做得最好"时的回答

> "我认为做得最好的是 **架构的清晰分层**。application 只管装配和生命周期，compiler 只管编译，runtime 只管执行，adapter 只管协议适配，integration 只管基础设施集成。每一层的职责都很单一，彼此通过明确定义的接口通信。这个设计不是在项目开始时一次性完成的，而是在三个迭代周期中逐步收敛到现在的状态。我们专门花了大量精力做全局状态治理和兼容层收敛，让框架从'能工作'变为'可维护'。"

### 被问到"这个项目有什么不足"时的回答

> "主要不足有两个。第一，学习曲线比较陡峭。对于习惯 Gin/Echo 那种简单路由注册的开发者来说，理解 Module/DI/Registry/Compiler 的概念需要一些时间。我们正在通过标准接入模板和更清晰的文档降低门槛。第二，测试框架还不够完善。虽然各子模块有独立测试，但跨协议的集成测试和端到端测试覆盖不够，后续需要重点加强。"

---

## 五、面试官可能追问的底层原理

### Go 技术栈相关问题

| 问题 | 简要回答 |
|------|---------|
| 为什么选 Go 而不是 Java/Python？ | 服务端高性能场景，Go 的 Goroutine 并发模型和编译速度更适合我们这种基础设施层框架 |
| go.work 和 go.mod replace 的区别？ | go.work 是开发期聚合工具，不影响发布；replace 是编译期指令，影响版本解析 |
| 泛型在项目中怎么用的？ | Container.GetByType[T]、MustGetByType[T] 等类型化取值 API，还有 Body[T]、Query[T] 等 Binding wrapper 类型 |
| 反射用得多么？性能怎么保证？ | DI 容器、参数绑定大量用了反射，但高频路径做了缓存（router matcher 的编译缓存、binding resolver 的缓存、handler reflect 结果的缓存）。高 QPS 场景下 HandlerContext 使用对象池复用 |
| 并发安全怎么处理的？ | Router 安装采用原子替换（atomic.Value），保证编译期全量切换而非增量更新。运行时请求处理是 goroutine 级别的并发，每个请求创建独立的 child container |
| Context 传播策略？ | Go 标准 context.Context 贯穿整个请求生命周期，用于超时、取消、链路追踪的传递。框架层 Metadata 用于业务透传 |

### 架构设计广度问题

| 问题 | 简要回答 |
|------|---------|
| 对比 Spring Boot | Spring Boot 是 Java 生态的巨型框架，优点是生态丰富，缺点是启动慢、配置复杂、学习曲线陡。lania-g 更轻量，启动毫秒级，核心概念更少。但生态远不如 Spring 丰富 |
| 对比 Kratos | Kratos 是微服务框架，强依赖 protobuf 和服务治理。lania-g 更通用，不绑定于 protobuf，支持更多协议 |
| 对比 NestJS | NestJS 的 Module/Provider/DI 模型对我们的设计影响很大。但 NestJS 是 Node.js 框架，lania-g 是 Go。我们在模块化思想上借鉴了 NestJS，但实现上结合了 Go 的类型系统和多模块工作区特性 |
| 适配器模式的应用 | adapter/* 是典型的适配器模式，每个 adapter 把协议原生请求适配为统一的 HandlerContext，把协议差异隔离在 adapter 内部 |

---

## 六、系统设计面试题延展

面试官可能会让你根据 lania-g 的架构经验来设计系统，以下是几个常见场景：

### 场景 1：设计一个 API 网关

> 结合 lania-g 的经验，API 网关可以复用类似的架构：① 路由层支持多协议（HTTP/REST + gRPC + WebSocket）。② 插件/AOP 机制统一管理认证、限流、日志、熔断。③ 编译期配置校验，防止网关配置错误导致线上故障。④ 实例级配置隔离，不同租户/业务线使用独立的配置实例。

### 场景 2：设计一个多协议微服务框架

> 可以复用 lania-g 的核心设计：① 声明式路由 + 编译期校验。② 统一运行时执行引擎。③ 模块化 DI + 可拔插集成。④ 编译诊断 + 启动报告保障可靠性。⑤ 实例级 Registry 支持多应用实例共存（测试、金丝雀发布等场景）。

### 场景 3：设计一个可扩展的消息队列消费框架

> 可以复用 lania-g 的 MQ adapter 设计模式：① DSL 声明式定义 Consumer 和 Topic 绑定。② 编译期检查消费组冲突和序列化配置。③ 统一的 AOP 链路（消息预处理、重试、死信处理）。④ 通过 Integrations 对接不同的 MQ 实现（Kafka / RabbitMQ / RocketMQ）。

---

## 七、项目关键技术细节速查表

| 概念 | 一句话解释 | 代码位置 |
|------|-----------|---------|
| Application | 应用装配入口，持有 Runtime/Registry/Adapters | `application/v3/application.go` |
| Module | 资源组织单位，含 DI 容器、可见性控制 | `kernel/v3/module/` |
| Container | Provider 实例承载者，支持父子容器 | `kernel/v3/di/` |
| Registry | 编译前声明存储（路由、AOP、Binding） | `kernel/v3/registry/` |
| Compiler | 读取 Registry，调用 Plugin 编译 | `kernel/v3/compiler/` |
| Runtime | 协议无关的统一执行引擎 | `kernel/v3/runtime/` |
| ProtocolPlugin | 协议编译插件接口 | 各 `protocol/*/v3/plugin.go` |
| HandlerContext | 统一请求上下文抽象 | `kernel/v3/runtime/context.go` |
| Pipeline | AOP 执行链 | `kernel/v3/runtime/pipeline.go` |
| Adapter | 协议 DSL + Transport + Plugin | `protocol/*/v3/` |
| Integrations | 基础设施模块化接入 | `integrations/*/v3/` |
| CompileDiagnostics | 编译诊断快照 | `application/v3/` |
| StartupReport | 启动报告输出 | `application/v3/` |

---

## 八、项目涉及的 Go 语言技术点（面试亮点）

1. **多模块工作区（Go Workspace）**：管理 20+ 子模块的协同开发
2. **泛型**：Container.GetByType[T]、Body[T] 等
3. **反射**：DI 容器、参数绑定、AOP 处理器发现
4. **接口设计**：ProtocolPlugin、BindingResolver、Lifecycle 等框架级接口
5. **并发模型**：goroutine-per-request、atomic.Value 原子替换 Router、对象池
6. **Context 传播**：Go 标准 context 的 timeout/cancel/trace 贯穿
7. **依赖注入**：自研 DI 容器，支持父子容器、作用域、token/type 双模式
8. **编译时检查 vs 运行时检查**：routeKey 冲突编译期报错，避免运行时静默覆盖
9. **插件模式**：ProtocolPlugin 接口允许动态扩展协议支持
10. **模块化设计**：integration 模块统一 API 模式（ForRoot / Factory / InjectXxx）

---

## 九、架构图（面试时可以在白板上画出来）

```
┌──────────────────────────────────────────────────────────────┐
│                      Application                             │
│  ┌──────────┐  ┌──────────┐  ┌───────────────────────────┐  │
│  │  Module  │  │ Registry │  │        Adapter            │  │
│  │  Loader  │  │ (声明池)  │  │  ┌──────┐ ┌──────┐      │  │
│  │  ──────  │  │          │  │  │ DSL  │ │Plugin│      │  │
│  │ ModuleRef│  │  Routes  │  │  └──────┘ └──────┘      │  │
│  │ Container│  │  AOP     │  │  ┌──────────┐           │  │
│  │ Tree     │  │  Binding │  │  │Transport │           │  │
│  └──────────┘  └──────────┘  │  └──────────┘           │  │
│                              └───────────────────────────┘  │
│         ↓ Compile(Registry + Plugins) → CompiledApp         │
│         ↓ Install(Runtime)                                   │
│  ┌──────────────────────────────────────────────────┐        │
│  │                   Runtime                        │        │
│  │  ┌────────┐ ┌──────────┐ ┌──────┐ ┌──────────┐ │        │
│  │  │ Router │ │Binding   │ │      │ │Pipeline  │ │        │
│  │  │ Match  │ │Registry  │ │Executor││Middlewares│ │        │
│  │  │        │ │          │ │      │ │Guards    │ │        │
│  │  └────────┘ └──────────┘ │      │ │Pipes     │ │        │
│  │                          │      │ │Interceptor│ │        │
│  │                          │      │ │Filters   │ │        │
│  │                          └──────┘ └──────────┘ │        │
│  └──────────────────────────────────────────────────┘        │
│              ↓ Request → Handler → Response                  │
└──────────────────────────────────────────────────────────────┘

接入方式：
  ├─ integrations/orm      ORM + Repository
  ├─ integrations/logger   结构化日志
  ├─ integrations/cache    缓存（内存/Redis）
  ├─ integrations/otel     可观测性
  ├─ integrations/auth     认证鉴权
  ├─ integrations/resilience 弹性策略
  ├─ integrations/kafka    Kafka 消息
  └─ integrations/mongodb  MongoDB
```

---

## 十、总结：面试中务必传达的几个点

1. **架构设计能力**：三段式架构（声明→编译→执行）是你自己的设计思考
2. **复杂问题治理经验**：全局状态治理、兼容层收敛、渐进式改造
3. **Go 技术深度**：多模块工作区、泛型、反射、DI、并发模型
4. **工程化思维**：编译期防御、诊断工具、渐进式收敛、兼容性管理
5. **系统设计视野**：多协议统一治理、适配器模式、插件模式、模块化设计