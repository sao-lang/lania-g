---
title: 快速开始
weight: 2
prev: /docs/getting-started/installation
next: /docs/getting-started/project-structure
---

本章用最小代码跑通一个 HTTP 服务，并解释每一步在做什么。

## 1. 写业务代码

`lania-g` 不要求嵌入特定基类，普通 Go struct + 方法就是 Controller。
路由声明写在控制器自己的 `Register` 方法里：

```go
package main

import (
	"log"
	"os"

	application "github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
)

type UserController struct{}

func (c *UserController) Ping() map[string]string {
	return map[string]string{"ok": "true"}
}

// Register 等价于 NestJS 的 @Controller + @Get 元数据块。
func (c *UserController) Register(r *httpadapter.Routes) {
	r.Prefix("/users").
		Get("/ping", c.Ping).
		Summary("Ping").
		Tags("users")
}

func main() {
	// 1) 根模块是唯一的装配入口
	root := module.New(module.Metadata{
		Imports: []module.Module{
			// 2) 协议也是模块
			httpadapter.Module(httpadapter.Config{Addr: ":8080"}),
		},
		Controllers: []any{&UserController{}},
	})

	// 3) 一个构造入口
	app, err := application.New(root, application.Options{Logger: os.Stdout})
	if err != nil {
		log.Fatal(err)
	}

	// 4) 一个运行入口：编译、启动、阻塞
	if err := app.Run(); err != nil {
		log.Fatal(err)
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
| `module.New(Metadata)` | 声明模块树、providers、controllers、AOP | [Module](../core-concepts/module) |
| `httpadapter.Module(Config)` | 把 HTTP 适配器装配成模块 | [HTTP](../protocols/http) |
| `Controller.Register(*Routes)` | 在控制器内声明路由，归属所在模块 | [编译与执行](../core-concepts/compile-runtime) |
| `application.New` | 加载模块树、发现并挂载适配器 | [依赖注入](../core-concepts/di) |
| `app.Run()` | 声明阶段 → 编译 → 启动 → 阻塞 | [编译与执行](../core-concepts/compile-runtime) |

{{< callout type="info" >}}
`app.Run()` 会阻塞直到 `app.Stop()` 被调用；监听地址来自 `http.Config{Addr}`。
{{< /callout >}}

## 4. 下一步

- 想了解目录怎么划分 → [项目结构](project-structure)
- 想加带鉴权的完整 CRUD → [构建一个带鉴权的 REST 服务](rest-service)
- 想换成 gRPC / GraphQL / WebSocket → [协议指南](../protocols/http)
