package main

import (
	"os"

	graphqladapter "github.com/sao-lang/lania-g/protocol/graphql/v3"
	"github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

type QueryResolver struct{}

func (r *QueryResolver) Ping() (map[string]string, error) {
	return map[string]string{"ok": "true"}, nil
}

func main() {
	query := &QueryResolver{}

	// GraphQL 的 owner 推导依赖 module resolvers 槽位。
	root := module.CreateModule(nil, nil, nil, []any{query}, nil)

	gqlAdapter := graphqladapter.New(":8080").WithPlayground(true)
	app, err := application.NewWithOptions(root, application.Options{
		Registry:        application.NewRegistry(),
		StartupReporter: os.Stdout,
	}, gqlAdapter)
	if err != nil {
		panic(err)
	}

	gqlAPI := gqlAdapter.API().(*graphqladapter.API)
	builder := gqlAPI.Resolver("Query", query)
	builder.Query("ping", query.Ping)
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
