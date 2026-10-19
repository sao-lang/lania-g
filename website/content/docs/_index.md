---
title: 文档
weight: 1
next: getting-started
toc: false
---

欢迎来到 `lania-g` 文档。

本站按照「先跑起来 → 理解核心概念 → 按协议查阅 → 深入原理 → 最佳实践」的顺序组织，
建议第一次接触时从 [快速开始](getting-started/quick-start) 读起。

## 阅读路径

| 阶段 | 章节 | 目标 |
|------|------|------|
| 入门 | [安装](getting-started/installation) · [快速开始](getting-started/quick-start) · [项目结构](getting-started/project-structure) | 5 分钟跑通一个 HTTP 服务 |
| 全景 | [特性地图](features) | 一次看清框架提供的所有能力 |
| 核心 | [Module](core-concepts/module) · [依赖注入](core-concepts/di) · [编译与执行](core-concepts/compile-runtime) · [AOP](core-concepts/aop) · [Binding](core-concepts/binding) | 理解框架如何组织代码与处理请求 |
| 完整流程 | [HTTP 全流程](guides/full-flow-http) · [多协议](guides/multi-protocol) · [部署](guides/deployment) | 从 main 到响应的每一跳 |
| 协议 | [HTTP](protocols/http) · [gRPC](protocols/grpc) · [GraphQL](protocols/graphql) · [WebSocket](protocols/websocket) · [MQ](protocols/mq) · [Scheduler](protocols/scheduler) | 按需接入具体协议 |
| 参考手册 | [Application](reference/application) · [Module & DI](reference/module-di) · [Registry/Compiler/Runtime](reference/registry-compiler-runtime) · [AOP](reference/aop) · [Binding](reference/binding) · [HTTP](reference/http) · [gRPC](reference/grpc) · [GraphQL](reference/graphql) · [WebSocket](reference/websocket) · [MQ](reference/mq) · [Scheduler](reference/scheduler) · [集成](reference/integrations) · [错误模型](reference/errors) | 全量 API 与语法 |
| 速查 | [语法速查](syntax) | 一页看全所有 DSL |
| 深入 | [请求执行链](internals/request-flow) · [Adapter 扩展](internals/adapter-plugin) · [诊断能力](internals/diagnostics) | 看懂源码与扩展点 |
| 实践 | [错误与生命周期](core-concepts/errors-lifecycle) · [测试策略](best-practices/testing) · [常见问题](best-practices/troubleshooting) | 上线前的最后一公里 |

## 约定

本文档中的示例均来自 `cmd/` 下已通过端到端验证的示例工程：

- `cmd/http-demo`、`cmd/grpc-demo`、`cmd/graphql-demo`、`cmd/ws-demo`
- 所有示例都支持 `PORT` / `APP_ADDR` / `GRPC_ADDR` 环境变量覆盖监听地址

{{< callout type="info" >}}
**版本**：文档对应 `v3` 系列（`application/v3`、`kernel/v3`、`protocol/*/v3`、`integrations/*/v3`）。
{{< /callout >}}
