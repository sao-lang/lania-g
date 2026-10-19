package main

import (
	"log"
	"os"

	application "github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/cmd/internal/addr"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	wsadapter "github.com/sao-lang/lania-g/protocol/ws/v3"
)

type ChatGateway struct{}

func (g *ChatGateway) Ping() (map[string]string, error) {
	return map[string]string{"ok": "true"}, nil
}

func main() {
	gateway := &ChatGateway{}

	wsModule := wsadapter.Module(wsadapter.Config{Addr: addr.HTTP(":8080")})
	root := module.New(module.Metadata{
		Imports:     []module.Module{wsModule},
		Controllers: []any{gateway},
	})

	app, err := application.New(root, application.Options{Logger: os.Stdout})
	if err != nil {
		log.Fatal(err)
	}

	wsAPI := wsModule.API()
	builder := wsAPI.Gateway("/ws/chat", gateway)
	builder.On("ping", gateway.Ping)
	if _, err := builder.BuildE(); err != nil {
		log.Fatal(err)
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
