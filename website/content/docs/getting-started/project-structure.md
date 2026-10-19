---
title: 项目结构
weight: 3
prev: /docs/getting-started/quick-start
next: /docs/getting-started/rest-service
---

`lania-g` 是一个 Go 多模块工作区，每个目录独立演进、独立发布。

## 顶层布局

```text
lania-g/
├── application/v3     应用装配、生命周期、编译诊断
├── kernel/v3          协议无关内核
│   ├── module/        模块定义、导入导出、递归加载
│   ├── di/            依赖注入容器
│   ├── registry/      声明存储（路由、AOP、Binding）
│   ├── compiler/      声明扫描、编译、冲突检测
│   ├── runtime/       路由匹配、参数绑定、Pipeline 执行
│   ├── aop/           Middleware/Guard/Pipe/Interceptor/Filter
│   ├── errors/        内核错误模型
│   ├── graph/         依赖图与归属分析
│   ├── integration/   Integration 注入桥接
│   └── adapter/       Application 与 Adapter 契约
├── protocol/          协议适配器（DSL + Transport + 编译插件）
│   ├── http/v3
│   ├── grpc/v3
│   ├── graphql/v3
│   ├── ws/v3
│   ├── mq/v3
│   └── scheduler/v3
├── integrations/      基础设施集成
├── cmd/               示例工程 + 内部工具
└── docs/              设计文档与执行台账
```

## 分层职责

| 层 | 包 | 职责 | 业务是否应依赖 |
|----|----|------|----------------|
| 应用层 | `application/v3` | 装配模块、挂载 adapter、编译、启动 | ✅ 推荐 |
| 内核层 | `kernel/v3/*` | 模块、DI、Registry、Compiler、Runtime | 部分（`module`、`di`） |
| 协议层 | `protocol/*/v3` | 协议 DSL、Transport、Binding | ✅ 推荐 |
| 集成层 | `integrations/*/v3` | 日志、ORM、认证等基础设施 | ✅ 按需 |
| 示例层 | `cmd/*` | 可运行示例 | ❌ 不要当生产骨架 |

## 依赖边界建议

业务代码推荐只依赖：

```text
github.com/sao-lang/lania-g/application/v3
github.com/sao-lang/lania-g/kernel/v3/module
github.com/sao-lang/lania-g/kernel/v3/di
github.com/sao-lang/lania-g/protocol/<proto>/v3
github.com/sao-lang/lania-g/integrations/<name>/v3
```

`compiler`、`runtime`、`registry` 属于内部基础设施：扩展框架时使用，普通业务不需要直接引用。

## 多模块与 go.work

`go.work` 把各子模块串成一个工作区，因此：

- 示例 `cmd` 可以直接引用本地模块（无需发布版本）
- 每个子模块仍然可以单独 `go test ./...`
- 根目录**不能**执行 `go build ./...`（根目录没有 `go.mod`）

{{< callout type="warning" >}}
在根目录执行 `go build ./...` 会报
`directory prefix . does not contain modules listed in go.work`，这是预期行为。
{{< /callout >}}
