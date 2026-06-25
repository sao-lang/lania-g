# lania-g v3

`lania-g` 当前已经从旧的单 `v3` 聚合包切换为多 module 结构。业务项目不再需要因为接入某个协议或某个 integration，而被动依赖整棵 `v3` 大目录。

当前文档中的 import 示例统一使用真实 module path `github.com/sao-lang/lania-g/...`。

## 模块布局

仓库的推荐依赖面如下：

- `github.com/sao-lang/lania-g/application/v3`：应用装配与生命周期
- `github.com/sao-lang/lania-g/kernel/v3/module`、`github.com/sao-lang/lania-g/kernel/v3/di`：模块与依赖注入
- `github.com/sao-lang/lania-g/protocol/<proto>/v3`：协议模块，例如 HTTP、gRPC、GraphQL、WS、MQ、Scheduler
- `github.com/sao-lang/lania-g/protocol/<proto>/v3/binding`：各协议绑定类型
- `github.com/sao-lang/lania-g/integrations/<name>/v3`：基础设施集成，例如 logger、config、orm、otel、swagger

`docs/` 目录现在承载文档，`cmd/` 目录现在承载示例工程；旧 `v3/` 聚合目录已经移除。

## 快速开始

仓库根目录的 `go.work` 已经挂载新的多 module 目录。推荐在目标模块目录内执行 `go` 命令，例如：

```bash
cd application/v3
go test ./...
```

```bash
cd protocol/http/v3
go test ./...
```

```bash
cd integrations/logger/v3
go test ./...
```

如果你在做业务接入，通常只需要关注 `application/v3`、目标协议模块以及你实际使用到的 integrations。

## 推荐接入路径

1. 使用 `github.com/sao-lang/lania-g/application/v3` 创建应用实例。
2. 使用 `github.com/sao-lang/lania-g/kernel/v3/module` 组织根模块与 imports/providers/controllers。
3. 按需挂载协议模块，例如 `github.com/sao-lang/lania-g/protocol/http/v3`。
4. 按需引入 integrations，例如 logger、config、orm、otel。
5. 启动前执行 `CompileDiagnostics()` 与 `StartupReport()` 做诊断。

## 核心链路

当前多 module 版本的主线链路仍然是“声明 -> 编译 -> 安装 -> 启动/执行”：

```text
Module
  -> Application
  -> Protocol Module 挂载
  -> Compile / Install
  -> Listen / Run
```

请求期统一进入 runtime 执行：

```text
client request
  -> protocol transport
  -> runtime.HandlerContext
  -> runtime.Router.Match
  -> binding args resolve
  -> AOP pipeline
  -> invoke handler
  -> write response
```

## 文档索引

- 文档导航：[docs/README.md](docs/README.md)
- API 文档：[docs/API.md](docs/API.md)
- 模块与链路全景说明：[docs/模块与链路全景说明.md](docs/模块与链路全景说明.md)
- 技术方案：[docs/技术方案.md](docs/技术方案.md)
- 标准接入模板：[docs/标准接入模板.md](docs/标准接入模板.md)
- 学习路线图：[docs/学习路线图.md](docs/学习路线图.md)
- 架构收敛文档：[docs/架构收敛文档.md](docs/架构收敛文档.md)
- Runtime 说明：[docs/runtime.md](docs/runtime.md)
- 错误模型约定：[docs/错误模型约定.md](docs/错误模型约定.md)
