---
title: 协议指南
weight: 3
sidebar:
  open: true
toc: false
---

同一个 Runtime 之上，每个协议只负责「传输 + 参数绑定」：

- [HTTP](http)：REST、参数校验、Swagger、静态资源、SSE
- [gRPC](grpc)：unary 与三种流模式
- [GraphQL](graphql)：schema DSL、嵌套选择集、guard
- [WebSocket](websocket)：Socket.IO v4 网关、房间与广播
- [MQ](mq)：消费者、topic 订阅、ack/nack
- [Scheduler](scheduler)：周期任务、重试、并发控制、快照

{{< callout type="info" >}}
所有协议共用同一套 Module / DI / AOP / Binding 语义，差异只在 wrapper 类型与响应编码。
{{< /callout >}}
