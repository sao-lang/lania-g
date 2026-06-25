package grpc

import (
	stdctx "context"
	"io"
	"net"
	"reflect"
	"strings"
	"testing"

	coredi "github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"

	gogrpc "google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

func newTestConfig(t *testing.T) (Config, func()) {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := gogrpc.NewServer()
	go func() {
		_ = server.Serve(listener)
	}()

	cfg := Config{
		Target:   "bufnet",
		Insecure: true,
		DialOptions: []gogrpc.DialOption{
			gogrpc.WithContextDialer(func(ctx stdctx.Context, target string) (net.Conn, error) {
				return listener.Dial()
			}),
		},
	}
	cleanup := func() {
		server.Stop()
		_ = listener.Close()
	}
	return cfg, cleanup
}

func newStreamTestConfig(t *testing.T) (Config, func()) {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := gogrpc.NewServer()
	server.RegisterService(&gogrpc.ServiceDesc{
		ServiceName: "test.StreamService",
		HandlerType: (*any)(nil),
		Streams: []gogrpc.StreamDesc{
			{
				StreamName:    "Echo",
				ClientStreams: true,
				ServerStreams: true,
				Handler: func(srv any, stream gogrpc.ServerStream) error {
					for {
						var msg emptypb.Empty
						err := stream.RecvMsg(&msg)
						if err == io.EOF {
							return nil
						}
						if err != nil {
							return err
						}
						if err := stream.SendMsg(&emptypb.Empty{}); err != nil {
							return err
						}
					}
				},
			},
		},
	}, nil)
	go func() {
		_ = server.Serve(listener)
	}()

	cfg := Config{
		Target:   "bufnet-stream",
		Insecure: true,
		DialOptions: []gogrpc.DialOption{
			gogrpc.WithContextDialer(func(ctx stdctx.Context, target string) (net.Conn, error) {
				return listener.Dial()
			}),
		},
	}
	cleanup := func() {
		server.Stop()
		_ = listener.Close()
	}
	return cfg, cleanup
}

func TestForRoot_RegistersGRPCClientConnFactoryAndConfig(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	cfg, cleanup := newTestConfig(t)
	defer cleanup()

	mod, err := ForRoot(cfg)
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(interface{ SetRegistry(*registry.Registry) }).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	defer mod.Destroy()

	client, err := coredi.GetByType[*Client](mod.Container())
	if err != nil {
		t.Fatalf("get client: %v", err)
	}
	if client.Conn() == nil {
		t.Fatalf("conn is nil")
	}

	conn, err := coredi.GetByType[*gogrpc.ClientConn](mod.Container())
	if err != nil {
		t.Fatalf("get conn: %v", err)
	}
	if conn != client.Conn() {
		t.Fatalf("conn mismatch")
	}

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}
	ctx := runtime.NewHandlerContext("test")
	ctx.WithContext(stdctx.Background())
	ctx.Container = mod.Container().NewChild()

	value, err := br.Resolve(ctx, nil, reflect.TypeFor[*Client](), 0)
	if err != nil {
		t.Fatalf("resolve client: %v", err)
	}
	if value.(*Client) != client {
		t.Fatalf("resolved client mismatch")
	}

	factoryToken := reflect.TypeFor[Factory]()
	factoryValue, err := br.Resolve(ctx, nil, factoryToken, 1)
	if err != nil {
		t.Fatalf("resolve factory: %v", err)
	}
	factory := factoryValue.(Factory)
	derived, err := factory.New(cfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer derived.Close()
	if derived.Conn() == nil {
		t.Fatalf("derived conn nil")
	}
}

func TestForRoot_InitRequiresExplicitRegistry(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	cfg, cleanup := newTestConfig(t)
	defer cleanup()

	mod, err := ForRoot(cfg)
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	if err := mod.Init(); err == nil {
		t.Fatalf("expected missing registry error")
	} else if !strings.Contains(err.Error(), "requires an explicit registry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestForRootCompat_InitRoutesToCompatFallbackSource(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	cfg, cleanup := newTestConfig(t)
	defer cleanup()

	mod, err := ForRootCompat(cfg)
	if err != nil {
		t.Fatalf("for root compat: %v", err)
	}
	if err := mod.Init(); err != nil {
		t.Fatalf("init compat module: %v", err)
	}
	if got := registry.Global().SnapshotFallbackUsage()["integrations/grpc.ForRootCompat"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}

func TestClient_NewStream(t *testing.T) {
	cfg, cleanup := newStreamTestConfig(t)
	defer cleanup()

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer client.Close()

	stream, err := client.NewStream(stdctx.Background(), &gogrpc.StreamDesc{
		StreamName:    "Echo",
		ClientStreams: true,
		ServerStreams: true,
	}, "/test.StreamService/Echo")
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	if err := stream.SendMsg(&emptypb.Empty{}); err != nil {
		t.Fatalf("send msg: %v", err)
	}
	var out emptypb.Empty
	if err := stream.RecvMsg(&out); err != nil {
		t.Fatalf("recv msg: %v", err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("close send: %v", err)
	}
}
