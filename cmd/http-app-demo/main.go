package main

import (
	"os"

	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
	"github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

type UserController struct{}

func (c *UserController) Ping() map[string]string {
	return map[string]string{"ok": "true"}
}

func main() {
	ctrl := &UserController{}
	root := module.CreateModule(nil, nil, []any{ctrl}, nil, nil)

	httpAdapter := httpadapter.New()
	app, err := application.NewWithOptions(root, application.Options{
		Registry:        application.NewRegistry(),
		StartupReporter: os.Stdout,
	}, httpAdapter)
	if err != nil {
		panic(err)
	}

	httpAPI := httpAdapter.API().(*httpadapter.API)
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
