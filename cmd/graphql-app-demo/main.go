package main

import (
	"log"
	"os"

	application "github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/cmd/internal/addr"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	graphqladapter "github.com/sao-lang/lania-g/protocol/graphql/v3"
)

type QueryResolver struct{}

func (r *QueryResolver) Ping() (map[string]string, error) {
	return map[string]string{"ok": "true"}, nil
}

// Declare 声明 GraphQL resolver 字段。
func (r *QueryResolver) Declare(routes *graphqladapter.Routes) {
	routes.Resolver("Query").Query("ping", r.Ping)
}

func main() {
	query := &QueryResolver{}

	gqlModule := graphqladapter.Module(graphqladapter.Config{Addr: addr.HTTP(":8080")})
	root := module.New(module.Metadata{
		Imports:   []module.Module{gqlModule},
		Resolvers: []any{query},
	})

	app, err := application.New(root, application.Options{Logger: os.Stdout})
	if err != nil {
		log.Fatal(err)
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
