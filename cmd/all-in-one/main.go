// all-in-one 演示四种协议在同一个应用、同一个运行时里共存：
//
//	HTTP + WebSocket + GraphQL 共享同一个监听端口，gRPC 独立监听。
//
// 运行：
//
//	cd cmd && go run ./all-in-one
//
// 可用环境变量覆盖默认地址：
//
//	APP_ADDR=:18080 GRPC_ADDR=:19051 go run ./all-in-one
//
// 验证入口：
//
//	GET  http://127.0.0.1:18080/hello                       HTTP
//	GET  http://127.0.0.1:18080/graphql?query={ping}        GraphQL
//	POST http://127.0.0.1:18080/socket.io/...               WebSocket（socket.io-client v4）
//	grpc :19051 allinone.Demo/Ping                          gRPC
package main

import (
	stdctx "context"
	"log"
	"os"

	application "github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/cmd/internal/addr"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	graphqladapter "github.com/sao-lang/lania-g/protocol/graphql/v3"
	grpcadapter "github.com/sao-lang/lania-g/protocol/grpc/v3"
	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
	wsadapter "github.com/sao-lang/lania-g/protocol/ws/v3"

	"google.golang.org/protobuf/types/known/emptypb"
)

// ---------------------------------------------------------------------------
// HTTP
// ---------------------------------------------------------------------------

// HTTPController 声明 HTTP 路由。
type HTTPController struct{}

// Hello 返回一个固定响应，用于验证 HTTP 端口。
func (c *HTTPController) Hello() (map[string]string, error) {
	return map[string]string{"protocol": "http"}, nil
}

// Declare 声明 HTTP 路由。
func (c *HTTPController) Declare(r *httpadapter.Routes) {
	r.Get("/hello", c.Hello)
}

// ---------------------------------------------------------------------------
// GraphQL
// ---------------------------------------------------------------------------

// QueryResolver 声明 GraphQL 查询字段。
type QueryResolver struct{}

// Ping 是一个最小的 GraphQL 查询。
func (q *QueryResolver) Ping() (map[string]string, error) {
	return map[string]string{"protocol": "graphql"}, nil
}

// Declare 把 ping 挂到 Query 上。
func (q *QueryResolver) Declare(r *graphqladapter.Routes) {
	r.Resolver("Query").Query("ping", q.Ping)
}

// ---------------------------------------------------------------------------
// WebSocket
// ---------------------------------------------------------------------------

// ChatGateway 声明 socket.io 事件。
type ChatGateway struct{}

// Ping 是 ping 事件的处理器。
func (g *ChatGateway) Ping() (map[string]string, error) {
	return map[string]string{"protocol": "ws"}, nil
}

// Declare 声明网关与事件。
func (g *ChatGateway) Declare(r *wsadapter.Routes) {
	r.Gateway("/ws/chat").On("ping", g.Ping)
}

// ---------------------------------------------------------------------------
// gRPC
// ---------------------------------------------------------------------------

// UserService 声明 gRPC 方法。
type UserService struct{}

type pingArgs struct {
	Ctx stdctx.Context
	Req *emptypb.Empty `req:"true" required:"true"`
}

// Ping 是一个最小的 unary 方法。
func (s *UserService) Ping(args pingArgs) (*emptypb.Empty, error) {
	_ = args.Ctx
	return args.Req, nil
}

// Declare 声明 gRPC 服务。服务名不带前导 `/`。
func (s *UserService) Declare(r *grpcadapter.Routes) {
	r.Service("allinone.Demo").Method("Ping", s.Ping)
}

func main() {
	httpModule := httpadapter.Module(httpadapter.Config{
		CORS: httpadapter.DefaultCorsConfig(),
	})
	wsModule := wsadapter.Module(wsadapter.Config{})
	gqlModule := graphqladapter.Module(graphqladapter.Config{Playground: true})
	grpcModule := grpcadapter.Module(grpcadapter.Config{Addr: addr.GRPC(":19051")})

	root := module.New(module.Metadata{
		Imports: []module.Module{httpModule, wsModule, gqlModule, grpcModule},
		Controllers: []any{
			&HTTPController{},
			&ChatGateway{},
			&UserService{},
		},
		Resolvers: []any{&QueryResolver{}},
	})

	app, err := application.New(root, application.Options{Logger: os.Stdout})
	if err != nil {
		log.Fatal(err)
	}

	// 共享监听地址：HTTP 承载 /，WS 挂 /socket.io/，GraphQL 挂 /graphql。
	if err := app.Listen(addr.HTTP(":18080")); err != nil {
		log.Fatal(err)
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
