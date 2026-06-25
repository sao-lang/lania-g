# lania-g

`lania-g` 是一个基于 **Go 多模块工作区** 的 Web 应用框架，采用 **声明-编译-执行** 三段式架构，以统一运行时承接 HTTP、gRPC、GraphQL、WebSocket、MQ、Scheduler 等多协议请求，并提供开箱即用的认证、日志、ORM、可观测性、弹性策略等基础设施集成。

## 核心理念

```text
声明 (DSL)  →  编译 (Compiler)  →  安装 (Runtime)  →  执行 (Handler)
```

框架将 **协议差异前移到编译与 Transport 层**，运行期统一复用 `runtime.HandlerContext → Router → Binding → AOP Pipeline → Executor`，保证扩展协议时无需侵入核心执行层。

### 三大设计原则

| 原则 | 说明 |
|------|------|
| **声明与执行分离** | 协议层 DSL 将路由、服务、AOP 等声明写入 Registry；编译阶段再转为 Runtime 可执行产物 |
| **协议差异外置** | HTTP、gRPC、GraphQL 等各自保留协议语义，但运行期共用同一套执行引擎 |
| **能力模块化集成** | 认证、日志、ORM、OTel 等基础设施通过 `integrations/*` 以模块方式装配，不耦合内核 |

### 执行链路

**启动期：**
```text
Root Module → Module Loader → Container Tree → Adapter Mount → Protocol Plugins
→ Compile(Registry) → Install(Runtime) → Listen / Run
```

**请求期：**
```text
Client Request → Protocol Transport → HandlerContext → Route Match
→ Binding Resolve Args → AOP Pipeline → Invoke Handler → Write Response
```

## 模块布局

本项目是一个 **Go 多模块工作区**（`go.work`），每个子目录独立演进、独立发布：

```
lania-g/
├── application/v3         应用装配、生命周期管理、编译诊断
├── kernel/v3              内核基础设施（协议无关）
│   ├── module/            模块定义、导入导出、递归加载
│   ├── di/                依赖注入容器（支持父子容器、作用域、类型/token 取值）
│   ├── registry/          声明存储（路由、AOP、Binding 等）
│   ├── compiler/          声明扫描、编译、冲突检测
│   ├── runtime/           统一运行期：路由匹配、参数绑定、Pipeline 执行
│   ├── aop/               Middleware、Guard、Pipe、Interceptor、Filter
│   ├── errors/            内核错误模型
│   ├── graph/             依赖图与归属分析
│   ├── scanner/           声明扫描辅助
│   ├── integration/       Integration 注入桥接
│   └── adapter/           Application 与 Adapter 契约
├── protocol/              协议适配器（DSL + Transport + 编译插件）
│   ├── http/v3/           HTTP 协议
│   ├── grpc/v3/           gRPC 协议（含四种流模式）
│   ├── graphql/v3/        GraphQL 协议
│   ├── ws/v3/             WebSocket 协议
│   ├── mq/v3/             消息队列协议
│   └── scheduler/v3/      定时调度协议
├── integrations/          基础设施集成（模块化装配）
│   ├── auth/v3/           认证鉴权
│   ├── cache/v3/          缓存（内存/Redis）
│   ├── config/v3/         配置加载
│   ├── es/v3/             Elasticsearch
│   ├── events/v3/         事件总线
│   ├── grpc/v3/           gRPC 客户端
│   ├── http/v3/           HTTP 客户端
│   ├── kafka/v3/          Kafka 消息队列
│   ├── logger/v3/         结构化日志
│   ├── minio/v3/          MinIO 对象存储
│   ├── mongodb/v3/        MongoDB（含 Repository 模式）
│   ├── orm/v3/            ORM（含事务）
│   ├── otel/v3/           OpenTelemetry 可观测性
│   ├── outbox/v3/         Outbox 发件箱模式
│   ├── resilience/v3/     弹性策略（熔断、限流、幂等）
│   ├── swagger/v3/        Swagger 文档自动生成
│   └── terminus/v3/       健康检查
├── cmd/                    示例工程（HTTP/gRPC/GraphQL/WS Demo）
└── docs/                   文档
```

## 快速开始

### 前置条件

- Go 1.25+

### 最小示例

```go
package main

import (
	application "github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	httpprotocol "github.com/sao-lang/lania-g/protocol/http/v3"
)

type UserController struct{}

func (c *UserController) Ping() map[string]string {
	return map[string]string{"ok": "true"}
}

func main() {
	ctrl := &UserController{}
	root := module.CreateModule(nil, nil, []interface{}{ctrl}, nil, nil)

	httpModule := httpprotocol.New()
	app, err := application.NewWithOptions(root, application.Options{
		Registry: application.NewRegistry(),
	}, httpModule)
	if err != nil {
		panic(err)
	}

	httpAPI := httpModule.API().(*httpprotocol.API)
	builder := httpAPI.Controller("/users", ctrl)
	builder.Get("/ping", ctrl.Ping)
	if _, err := builder.BuildE(); err != nil {
		panic(err)
	}

	if _, err := app.CompileDiagnostics(); err != nil {
		panic(err)
	}
	if _, err := app.StartupReport(); err != nil {
		panic(err)
	}
	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
```

### 运行测试

```bash
cd application/v3 && go test ./...
cd protocol/http/v3 && go test ./...
cd integrations/logger/v3 && go test ./...
```

## 推荐接入路径

1. 使用 `application/v3` 创建应用实例
2. 使用 `kernel/v3/module` 组织根模块与 imports/providers/controllers
3. 按需挂载协议模块，如 `protocol/http/v3`
4. 按需引入 integrations，如 `integrations/logger/v3`、`integrations/orm/v3`
5. 启动前执行 `CompileDiagnostics()` 与 `StartupReport()` 做诊断

## 文档索引

- [文档导航](docs/README.md)
- [API 文档](docs/API.md)
- [模块与链路全景说明](docs/模块与链路全景说明.md)
- [技术方案](docs/技术方案.md)
- [标准接入模板](docs/标准接入模板.md)
- [学习路线图](docs/学习路线图.md)
- [架构收敛文档](docs/架构收敛文档.md)
- [Runtime 说明](docs/runtime.md)
- [错误模型约定](docs/错误模型约定.md)

### 专项接入文档

- [Auth 接入](docs/auth接入.md)
- [MongoDB 接入](docs/mongodb接入.md)
- [OpenTelemetry 接入](docs/otel接入.md)
- [Outbox 接入](docs/outbox接入.md)
- [Resilience 接入](docs/resilience接入.md)
- [gRPC 四种模式支持方案](docs/grpc四种模式完整支持方案.md)

## 业务接入依赖边界

推荐业务层只依赖以下包族：

- `github.com/sao-lang/lania-g/application/v3`
- `github.com/sao-lang/lania-g/kernel/v3/module`
- `github.com/sao-lang/lania-g/kernel/v3/di`
- `github.com/sao-lang/lania-g/protocol/<proto>/v3`
- `github.com/sao-lang/lania-g/integrations/<name>/v3`

不建议将 `compiler`、`runtime`、`registry` 等内核包作为常规业务依赖。

## License

[MIT](LICENSE)