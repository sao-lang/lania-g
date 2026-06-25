package ws

import (
	"strings"
	"testing"

	wsbinding "github.com/sao-lang/lania-g/protocol/ws/v3/binding"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	wsprotocol "github.com/sao-lang/lania-g/protocol/ws/v3/protocol"
)

type testGateway struct{}

type echoDTO struct {
	Msg string `json:"msg"`
}

func (g *testGateway) Echo(args wsbinding.WSMessageBody[echoDTO]) (any, error) {
	return args.Value.Msg, nil
}

type echoStruct struct {
	Msg string `json:"msg"`
}

func (g *testGateway) EchoStruct(args echoStruct) (any, error) {
	return args.Msg, nil
}

func (g *testGateway) EchoCtx(ctx wsbinding.Context) (any, error) {
	return map[string]any{"event": ctx.Event(), "ns": ctx.Namespace()}, nil
}

func (g *testGateway) EchoComposite(args struct {
	Token wsbinding.WSHeaders[string]     `header:"Authorization" required:"true"`
	ID    wsbinding.WSMessageBody[string] `body:"id" required:"true"`
}) (any, error) {
	return map[string]any{"token": args.Token.Value, "id": args.ID.Value}, nil
}

type dtoWithHeader struct {
	Msg   string `json:"msg"`
	Token string `header:"Authorization"`
}

func (g *testGateway) EchoAutoStruct(args dtoWithHeader) (any, error) {
	return map[string]any{"msg": args.Msg, "token": args.Token}, nil
}

func TestPluginScan_RequiresExplicitRegistry(t *testing.T) {
	gw := &testGateway{}
	root := module.CreateModule(nil, nil, []any{gw}, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)

	reg := registry.New()
	NewAPI(reg).Gateway("/ws/users", gw).On("echo", gw.Echo).Build()

	_, err := (&Plugin{}).Scan(moduleRef, nil)
	if err == nil {
		t.Fatalf("expected missing registry error")
	}
	if !strings.Contains(err.Error(), "requires an explicit registry") {
		t.Fatalf("unexpected error: %v", err)
	}

	compiled, err := compiler.Compile(moduleRef, reg, NewPlugin())
	if err != nil {
		t.Fatalf("compile with explicit registry: %v", err)
	}
	if compiled == nil {
		t.Fatalf("expected compiled app")
	}
}

func TestPlugin_CompileAndExecute(t *testing.T) {
	gw := &testGateway{}
	root := module.CreateModule(nil, nil, []any{gw}, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)

	reg := registry.New()
	b := NewAPI(reg).Gateway("/ws/users", gw)
	b.On("echo", gw.Echo)
	b.On("echo_struct", gw.EchoStruct)
	b.On("echo_ctx", gw.EchoCtx)
	b.On("echo_comp", gw.EchoComposite)
	b.On("echo_auto", gw.EchoAutoStruct)
	b.Build()

	compiled, err := compiler.Compile(moduleRef, reg, NewPlugin())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	rt := runtime.NewRuntime()
	if err := compiled.Install(rt); err != nil {
		t.Fatalf("install: %v", err)
	}

	{
		ctx := runtime.NewHandlerContext(wsprotocol.Protocol)
		ctx.Request.Method = "echo"
		ctx.Request.Path = "/ws/users"
		ctx.Request.BodyBytes = []byte(`{"msg":"hi"}`)

		out, err := rt.Execute(ctx)
		if err != nil {
			t.Fatalf("execute echo: %v", err)
		}
		if out != "hi" {
			t.Fatalf("out=%v want %v", out, "hi")
		}
	}
	{
		ctx := runtime.NewHandlerContext(wsprotocol.Protocol)
		ctx.Request.Method = "echo_struct"
		ctx.Request.Path = "/ws/users"
		ctx.Request.BodyBytes = []byte(`{"msg":"hello"}`)

		out, err := rt.Execute(ctx)
		if err != nil {
			t.Fatalf("execute echo_struct: %v", err)
		}
		if out != "hello" {
			t.Fatalf("out=%v want %v", out, "hello")
		}
	}
	{
		ctx := runtime.NewHandlerContext(wsprotocol.Protocol)
		ctx.Request.Method = "echo_ctx"
		ctx.Request.Path = "/ws/users"

		out, err := rt.Execute(ctx)
		if err != nil {
			t.Fatalf("execute echo_ctx: %v", err)
		}
		m, ok := out.(map[string]any)
		if !ok {
			t.Fatalf("out type = %T, want map", out)
		}
		if m["event"] != "echo_ctx" || m["ns"] != "/ws/users" {
			t.Fatalf("out=%v", out)
		}
	}
	{
		ctx := runtime.NewHandlerContext(wsprotocol.Protocol)
		ctx.Request.Method = "echo_comp"
		ctx.Request.Path = "/ws/users"
		ctx.Request.BodyBytes = []byte(`{"id":"42"}`)
		ctx.Request.Headers["Authorization"] = "tkn"
		ctx.Request.HeadersMulti["Authorization"] = []string{"tkn"}

		out, err := rt.Execute(ctx)
		if err != nil {
			t.Fatalf("execute echo_comp: %v", err)
		}
		m, ok := out.(map[string]any)
		if !ok {
			t.Fatalf("out type = %T, want map", out)
		}
		if m["token"] != "tkn" || m["id"] != "42" {
			t.Fatalf("out=%v", out)
		}
	}
	{
		ctx := runtime.NewHandlerContext(wsprotocol.Protocol)
		ctx.Request.Method = "echo_auto"
		ctx.Request.Path = "/ws/users"
		ctx.Request.BodyBytes = []byte(`{"msg":"ok"}`)
		ctx.Request.Headers["Authorization"] = "tkn2"
		ctx.Request.HeadersMulti["Authorization"] = []string{"tkn2"}

		out, err := rt.Execute(ctx)
		if err != nil {
			t.Fatalf("execute echo_auto: %v", err)
		}
		m, ok := out.(map[string]any)
		if !ok {
			t.Fatalf("out type = %T, want map", out)
		}
		if m["token"] != "tkn2" || m["msg"] != "ok" {
			t.Fatalf("out=%v", out)
		}
	}
}
