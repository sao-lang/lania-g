---
title: lania-g
layout: hextra-home
toc: false
---

<div class="lg-home">

<section class="lg-hero">
  <div class="lg-badge">v3 · Go 1.25+ · 全链路已端到端验证</div>

  <h1>
    声明 → 编译 → 执行
    <span class="lg-gradient">一套内核，承载所有协议</span>
  </h1>

  <p>
    用 Go 多模块工作区组织内核与协议适配器：HTTP、gRPC、GraphQL、WebSocket、MQ、Scheduler
    共享同一条 Runtime 执行链。协议差异只存在于 Transport 与 Binding 层。
  </p>

  <div class="lg-actions">
    <a class="lg-btn lg-btn--primary" href="/lania-g/docs/getting-started/quick-start/">快速开始 →</a>
    <a class="lg-btn" href="/lania-g/docs/guides/full-flow-http/">完整流程</a>
    <a class="lg-btn" href="/lania-g/docs/features/">特性总览</a>
  </div>

  <div class="lg-code">
    <div class="lg-code__bar">
      <span class="lg-code__dot"></span>
      <span class="lg-code__dot"></span>
      <span class="lg-code__dot"></span>
      <span class="lg-code__title">main.go</span>
    </div>
    <pre><code class="language-go">// 1. 控制器：路由声明与 handler 放在一起
func (c *UserController) Declare(r *http.Routes) {
    r.Prefix("/users").Get("/ping", c.Ping)
}

// 2. 根模块：唯一的装配入口
root := module.New(module.Metadata{
    Imports:     []module.Module{http.Module(http.Config{Addr: ":8080"})},
    Controllers: []any{&amp;UserController{}},
})

// 3. 启动：New + Run
app, _ := application.New(root, application.Options{Logger: os.Stdout})
app.Run()</code></pre>
  </div>
</section>

<section class="lg-section">
  <div class="lg-section__title">为什么是 lania-g</div>
  <div class="lg-grid">
    <a class="lg-card" href="/lania-g/docs/core-concepts/compile-runtime/">
      <div class="lg-card__title">声明与执行分离</div>
      <div class="lg-card__desc">DSL 只写声明，编译期统一校验与产物化，启动前就能发现冲突与缺失。</div>
      <span class="lg-card__tag">编译模型 →</span>
    </a>
    <a class="lg-card" href="/lania-g/docs/core-concepts/binding/">
      <div class="lg-card__title">协议感知的 Binding</div>
      <div class="lg-card__desc">按「参数类型 + 当前协议」双重匹配，多协议同进程不会互相抢占。</div>
      <span class="lg-card__tag">Binding 体系 →</span>
    </a>
    <a class="lg-card" href="/lania-g/docs/core-concepts/di/">
      <div class="lg-card__title">容器树与作用域</div>
      <div class="lg-card__desc">父子容器、Singleton/Request/Transient、字段注入与构造函数注入。</div>
      <span class="lg-card__tag">依赖注入 →</span>
    </a>
    <a class="lg-card" href="/lania-g/docs/core-concepts/aop/">
      <div class="lg-card__title">统一 AOP</div>
      <div class="lg-card__desc">Middleware / Guard / Pipe / Interceptor / Filter 五类增强，顺序固定可预期。</div>
      <span class="lg-card__tag">AOP →</span>
    </a>
    <a class="lg-card" href="/lania-g/docs/internals/diagnostics/">
      <div class="lg-card__title">启动期可诊断</div>
      <div class="lg-card__desc">CompileDiagnostics、StartupReport、GraphDiagnostics 三层自检。</div>
      <span class="lg-card__tag">诊断能力 →</span>
    </a>
    <a class="lg-card" href="/lania-g/docs/internals/adapter-plugin/">
      <div class="lg-card__title">可扩展协议</div>
      <div class="lg-card__desc">新增协议 = Adapter + Plugin，核心执行链零改动。</div>
      <span class="lg-card__tag">扩展指南 →</span>
    </a>
  </div>
</section>

<section class="lg-section">
  <div class="lg-section__title">六种协议，一条执行链</div>
  <div class="lg-grid">
    <a class="lg-card" href="/lania-g/docs/protocols/http/">
      <div class="lg-card__title">HTTP</div>
      <div class="lg-card__desc">路由 DSL、参数校验、CORS/Helmet、静态资源、Swagger。</div>
      <span class="lg-card__tag">REST</span>
    </a>
    <a class="lg-card" href="/lania-g/docs/protocols/grpc/">
      <div class="lg-card__title">gRPC</div>
      <div class="lg-card__desc">unary 与三种流模式，CompositeStruct 统一入参。</div>
      <span class="lg-card__tag">RPC</span>
    </a>
    <a class="lg-card" href="/lania-g/docs/protocols/graphql/">
      <div class="lg-card__title">GraphQL</div>
      <div class="lg-card__desc">schema DSL、嵌套选择集、嵌入字段摊平、guard。</div>
      <span class="lg-card__tag">Query</span>
    </a>
    <a class="lg-card" href="/lania-g/docs/protocols/websocket/">
      <div class="lg-card__title">WebSocket</div>
      <div class="lg-card__desc">Socket.IO v4 网关、事件后缀、房间与广播。</div>
      <span class="lg-card__tag">Realtime</span>
    </a>
    <a class="lg-card" href="/lania-g/docs/protocols/mq/">
      <div class="lg-card__title">MQ</div>
      <div class="lg-card__desc">消费者 DSL、topic 订阅、ack/nack、可插拔驱动。</div>
      <span class="lg-card__tag">Event</span>
    </a>
    <a class="lg-card" href="/lania-g/docs/protocols/scheduler/">
      <div class="lg-card__title">Scheduler</div>
      <div class="lg-card__desc">周期/cron 任务、重试、并发控制、运行快照。</div>
      <span class="lg-card__tag">Job</span>
    </a>
  </div>
</section>

<section class="lg-section">
  <div class="lg-section__title">按目标开始</div>
  <div class="lg-entries">
    <a class="lg-entry" href="/lania-g/docs/getting-started/installation/">
      <strong>第一次接触</strong>
      <span>环境准备 → 快速开始 → 项目结构</span>
    </a>
    <a class="lg-entry" href="/lania-g/docs/guides/full-flow-http/">
      <strong>要完整流程</strong>
      <span>模块 → DI → DSL → 编译 → 运行 → 诊断</span>
    </a>
    <a class="lg-entry" href="/lania-g/docs/reference/http/">
      <strong>要查 API</strong>
      <span>全量 DSL、参数绑定、Adapter 选项</span>
    </a>
    <a class="lg-entry" href="/lania-g/docs/syntax/">
      <strong>要查语法</strong>
      <span>一页看全模块 / DI / AOP / 全协议 DSL</span>
    </a>
    <a class="lg-entry" href="/lania-g/docs/best-practices/troubleshooting/">
      <strong>遇到问题</strong>
      <span>错误信息 → 根因 → 解决</span>
    </a>
  </div>
</section>

</div>
