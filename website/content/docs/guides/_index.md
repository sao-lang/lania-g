---
title: 完整流程
weight: 4
sidebar:
  open: true
toc: false
---

这里不是零散的知识点，而是**从空目录到可上线服务**的完整流程：

- [HTTP 完整流程](full-flow-http)：模块 → DI → DSL → 编译 → 运行 → 诊断 → 测试
- [多协议同进程](multi-protocol)：HTTP + WebSocket + GraphQL 共享监听
- [测试与部署](deployment)：端口配置、健康检查、优雅关闭、构建产物

建议阅读顺序：先完整走一遍 HTTP 流程，再看多协议如何组合。
