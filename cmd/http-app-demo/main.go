// http-app-demo 展示了 lania-g 的最小 HTTP 服务写法：
//
//	根模块 → 协议模块 → 控制器（在控制器里声明路由）
package main

import (
	"log"
	"os"

	application "github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/cmd/internal/addr"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
)

type UserController struct{}

func (c *UserController) Ping() map[string]string {
	return map[string]string{"ok": "true"}
}

// Register 声明本控制器的路由。
//
// 控制器只能通过注册到模块树的 Controllers 槽位生效，
// 因此路由天然归属于它所在的模块。
func (c *UserController) Register(r *httpadapter.Routes) {
	r.Prefix("/users").
		Get("/ping", c.Ping).
		Summary("Ping").
		Tags("users")
}

func main() {
	root := module.New(module.Metadata{
		Imports: []module.Module{
			httpadapter.Module(httpadapter.Config{Addr: addr.HTTP(":8080")}),
		},
		Controllers: []any{&UserController{}},
	})

	app, err := application.New(root, application.Options{Logger: os.Stdout})
	if err != nil {
		log.Fatal(err)
	}
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
