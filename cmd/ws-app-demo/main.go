package main

import (
	"os"

	wsadapter "github.com/sao-lang/lania-g/protocol/ws/v3"
	"github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

type ChatGateway struct{}

func (g *ChatGateway) Ping() (map[string]string, error) {
	return map[string]string{"ok": "true"}, nil
}

func main() {
	gateway := &ChatGateway{}

	// WS 的 owner 推导依赖 module controllers 槽位。
	root := module.CreateModule(nil, nil, []any{gateway}, nil, nil)

	wsAdapter := wsadapter.New(":8080")
	app, err := application.NewWithOptions(root, application.Options{
		Registry:        application.NewRegistry(),
		StartupReporter: os.Stdout,
	}, wsAdapter)
	if err != nil {
		panic(err)
	}

	wsAPI := wsAdapter.API().(*wsadapter.API)
	builder := wsAPI.Gateway("/ws/chat", gateway)
	builder.On("ping", gateway.Ping)
	if _, err := builder.BuildE(); err != nil {
		panic(err)
	}

	if _, err := app.CompileDiagnostics(); err != nil {
		panic(err)
	}
	if _, err := app.StartupReport(); err != nil {
		panic(err)
	}
	if err := app.Run(); err != nil {
		panic(err)
	}
}
