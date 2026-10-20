package e2e_test

import (
	"testing"

	"github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	graphqladapter "github.com/sao-lang/lania-g/protocol/graphql/v3"
	grpcadapter "github.com/sao-lang/lania-g/protocol/grpc/v3"
	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
	wsadapter "github.com/sao-lang/lania-g/protocol/ws/v3"
)

type httpCtrl struct{}

func (c *httpCtrl) Hello() (map[string]string, error) { return map[string]string{"p": "http"}, nil }

func (c *httpCtrl) Declare(r *httpadapter.Routes) { r.Get("/hello", c.Hello) }

type gqlResolver struct{}

func (r *gqlResolver) Ping() (map[string]string, error) {
	return map[string]string{"p": "graphql"}, nil
}

func (r *gqlResolver) Declare(routes *graphqladapter.Routes) {
	routes.Resolver("Query").Query("ping", r.Ping)
}

type wsGateway struct{}

func (g *wsGateway) Ping() (map[string]string, error) { return map[string]string{"p": "ws"}, nil }

func (g *wsGateway) Declare(r *wsadapter.Routes) { r.Gateway("/ws/chat").On("ping", g.Ping) }

// 四种协议装配到同一个应用：HTTP/WS/GraphQL 共享监听，gRPC 独立监听。
func TestMultiProtocolAssembly(t *testing.T) {
	root := module.New(module.Metadata{
		Imports: []module.Module{
			httpadapter.Module(httpadapter.Config{}),
			wsadapter.Module(wsadapter.Config{}),
			graphqladapter.Module(graphqladapter.Config{}),
			grpcadapter.Module(grpcadapter.Config{Addr: ":19051"}),
		},
		Controllers: []any{&httpCtrl{}, &wsGateway{}},
		Resolvers:   []any{&gqlResolver{}},
	})

	app, err := application.New(root, application.Options{})
	if err != nil {
		t.Fatalf("new application: %v", err)
	}
	if err := app.Listen(":18080"); err != nil {
		t.Fatalf("listen: %v", err)
	}

	report, err := app.StartupReport()
	if err != nil {
		t.Fatalf("startup report: %v", err)
	}
	if got := len(report.Protocols); got != 4 {
		t.Fatalf("protocols = %d, want 4", got)
	}
	if got := len(report.Adapters); got != 4 {
		t.Fatalf("adapters = %d, want 4", got)
	}
	if report.RouteCount != 3 {
		t.Fatalf("routes = %d, want 3", report.RouteCount)
	}
}
