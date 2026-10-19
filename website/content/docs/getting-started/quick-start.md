---
title: 快速开始
weight: 2
prev: /docs/getting-started/installation
next: /docs/getting-started/project-structure
---

本章用最小代码跑通一个 HTTP 服务，并解释每一步在做什么。

## 1. 写业务代码

`lania-g` 不要求嵌入特定基类，普通 Go struct + 方法就是 Controller：

```go
package main

import (
	"os"

	"github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/cmd/internal/addr"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
)

type UserController struct{}

func (c *UserController) Ping() map[string]string {
	return map[string]string{"ok": "true"}
}

func main() {
	ctrl := &UserController{}

	// 1) 把 Controller 放进模块的 controllers 槽位
	root := module.CreateModule(nil, nil, []any{ctrl}, nil, nil)

	// 2) 创建 HTTP adapter（共享监听模式）
	httpAdapter := httpadapter.New()

	// 3) 装配应用：显式注入实例级 Registry
	app, err := application.NewWithOptions(root, application.Options{
		Registry:        application.NewRegistry(),
		StartupReporter: os.Stdout,
	}, httpAdapter)
	if err != nil {
		panic(err)
	}

	// 4) 用 DSL 声明路由
	httpAPI := httpAdapter.API().(*httpadapter.API)
	builder := httpAPI.Controller("/users", ctrl)
	builder.Get("/ping", ctrl.Ping)
	if _, err := builder.BuildE(); err != nil {
		panic(err)
	}

	// 5) 启动前诊断 + 监听
	if _, err := app.CompileDiagnostics(); err != nil {
		panic(err)
	}
	if _, err := app.StartupReport(); err != nil {
		panic(err)
	}
	if err := app.Listen(addr.HTTP(":8080")); err != nil {
		panic(err)
	}
}
```

## 2. 运行

```bash
cd cmd
PORT=18080 go run ./http-app-demo
```

```bash
curl http://127.0.0.1:18080/users/ping
# {"ok":"true"}
```

## 3. 这五步分别做了什么

| 步骤 | 作用 | 相关章节 |
|------|------|----------|
| `module.CreateModule` | 声明模块树、providers、controllers | [Module](../core-concepts/module) |
| `httpadapter.New()` | 注册 HTTP 协议插件与 transport | [HTTP](../protocols/http) |
| `NewWithOptions` | 加载模块、构建容器树、挂载 adapter | [依赖注入](../core-concepts/di) |
| DSL `Controller(...).Get(...)` | 把路由声明写入 Registry | [编译与执行](../core-concepts/compile-runtime) |
| `CompileDiagnostics` + `Listen` | 编译声明、安装到 Runtime、开始监听 | [编译与执行](../core-concepts/compile-runtime) |

{{< callout type="info" >}}
`app.Listen(addr)` 会阻塞直到 `app.Stop()` 被调用，符合"长驻服务"直觉。
{{< /callout >}}

## 4. 下一步

- 想了解目录怎么划分 → [项目结构](project-structure)
- 想加带鉴权的完整 CRUD → [构建一个带鉴权的 REST 服务](rest-service)
- 想换成 gRPC / GraphQL / WebSocket → [协议指南](../protocols/http)
