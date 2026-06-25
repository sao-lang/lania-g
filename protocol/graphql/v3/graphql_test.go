package graphql

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	gqlbinding "github.com/sao-lang/lania-g/protocol/graphql/v3/binding"
	gqlprotocol "github.com/sao-lang/lania-g/protocol/graphql/v3/protocol"
)

type fakeHost struct {
	rt  *runtime.Runtime
	reg *registry.Registry
}

func (h *fakeHost) Runtime() *runtime.Runtime    { return h.rt }
func (h *fakeHost) Registry() *registry.Registry { return h.reg }
func (h *fakeHost) ModuleRef() *module.ModuleRef { return nil }

var _ adapter.Host = (*fakeHost)(nil)

type testResolver struct{}

type userDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type echoCompositeDTO struct {
	ID       string `json:"id"`
	Field    string `json:"field"`
	Vars     any    `json:"vars"`
	Selected int    `json:"selected"`
}

type validatedArgsDTO struct {
	ID string `json:"id" validate:"required,min=2"`
}

func (r *testResolver) User(id gqlbinding.Arg[string]) (userDTO, error) {
	return userDTO{ID: id.Value, Name: "alice"}, nil
}

func (r *testResolver) EchoComposite(args struct {
	Ctx  gqlbinding.Context
	ID   gqlbinding.Arg[string] `arg:"id"`
	Vars gqlbinding.Variables
	Set  gqlbinding.SelectionSet
}) (echoCompositeDTO, error) {
	return echoCompositeDTO{
		ID:       args.ID.Value,
		Field:    string(args.Ctx.FieldName()),
		Vars:     args.Vars["tenant"],
		Selected: len(args.Set.Fields),
	}, nil
}

func (r *testResolver) EchoAuto(args struct {
	ID string `json:"id"`
}) (any, error) {
	return map[string]any{"id": args.ID}, nil
}

func (r *testResolver) ValidateArgs(ctx gqlbinding.Context) (any, error) {
	var dto validatedArgsDTO
	if err := ctx.ShouldBindArgs(&dto); err != nil {
		return nil, err
	}
	return map[string]any{"id": dto.ID}, nil
}

func (r *testResolver) DisplayName(parent gqlbinding.Parent[userDTO], field gqlbinding.FieldName, set gqlbinding.SelectionSet) (string, error) {
	if field != "displayName" {
		return "", nil
	}
	if len(set.Fields) == 0 {
		return parent.Value.Name + "-display", nil
	}
	return parent.Value.Name + "-display", nil
}

func (r *testResolver) Unbound(first gqlbinding.Arg[string], second gqlbinding.Arg[string]) (string, error) {
	return first.Value + second.Value, nil
}

func buildRuntimeForTest(t *testing.T) (*runtime.Runtime, *registry.Registry) {
	t.Helper()
	resolver := &testResolver{}
	root := module.CreateModule(nil, nil, nil, []any{resolver}, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)
	reg := registry.New()

	NewAPI(reg).Resolver("User", resolver).
		Query("user", resolver.User).Arg("id").Returns("User").
		Query("echoComposite", resolver.EchoComposite).Arg("id").
		Query("echoAuto", resolver.EchoAuto).
		Query("validateArgs", resolver.ValidateArgs).Arg("id").
		Query("unbound", resolver.Unbound).
		Object("displayName", resolver.DisplayName).
		Build()

	compiled, err := compiler.Compile(moduleRef, reg, NewPlugin())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	rt := runtime.NewRuntime()
	if err := compiled.Install(rt); err != nil {
		t.Fatalf("install: %v", err)
	}
	return rt, reg
}

func TestPluginScan_RequiresExplicitRegistry(t *testing.T) {
	resolver := &testResolver{}
	root := module.CreateModule(nil, nil, nil, []any{resolver}, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)

	reg := registry.New()
	NewAPI(reg).Resolver("User", resolver).
		Query("user", resolver.User).
		Arg("id").
		Returns("User").
		Build()

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
	rt, _ := buildRuntimeForTest(t)

	{
		ctx := runtime.NewHandlerContext(gqlprotocol.Protocol)
		ctx.Request.Method = "Query"
		ctx.Request.Path = "user"
		ctx.Set(gqlbinding.MetadataKeyField, map[string]any{"id": "42"})

		out, err := rt.Execute(ctx)
		if err != nil {
			t.Fatalf("execute user: %v", err)
		}
		m, ok := out.(userDTO)
		if !ok {
			t.Fatalf("out type = %T, want userDTO", out)
		}
		if m.ID != "42" {
			t.Fatalf("out=%v", out)
		}
	}

	{
		ctx := runtime.NewHandlerContext(gqlprotocol.Protocol)
		ctx.Request.Method = "Query"
		ctx.Request.Path = "echoComposite"
		ctx.Set(gqlbinding.MetadataKeyField, map[string]any{"id": "c1"})
		ctx.Set(gqlbinding.MetadataKeyVars, map[string]any{"tenant": "t1"})
		gctx := &gqlbinding.GraphQLContext{}
		gqlbinding.InitContext(gctx, ctx.Context(), nil, nil, "", "", "Query", "echoComposite", nil, nil, nil, nil, nil, nil, nil, nil, nil)
		ctx.Set(gqlbinding.MetadataKeyContext, gctx)

		out, err := rt.Execute(ctx)
		if err != nil {
			t.Fatalf("execute echoComposite: %v", err)
		}
		m := out.(echoCompositeDTO)
		if m.ID != "c1" || m.Field != "echoComposite" || m.Vars != "t1" || m.Selected != 0 {
			t.Fatalf("out=%v", out)
		}
	}

	{
		ctx := runtime.NewHandlerContext(gqlprotocol.Protocol)
		ctx.Request.Method = "Query"
		ctx.Request.Path = "echoAuto"
		ctx.Set(gqlbinding.MetadataKeyField, map[string]any{"id": "auto"})

		out, err := rt.Execute(ctx)
		if err != nil {
			t.Fatalf("execute echoAuto: %v", err)
		}
		m := out.(map[string]any)
		if m["id"] != "auto" {
			t.Fatalf("out=%v", out)
		}
	}

	{
		ctx := runtime.NewHandlerContext(gqlprotocol.Protocol)
		ctx.Request.Method = "Query"
		ctx.Request.Path = "validateArgs"
		ctx.Set(gqlbinding.MetadataKeyField, map[string]any{"id": "ok"})
		gctx := &gqlbinding.GraphQLContext{}
		gqlbinding.InitContext(gctx, ctx.Context(), nil, nil, "", "", "Query", "validateArgs", nil, nil, nil, nil, nil, nil, nil, nil, map[string]any{"id": "ok"})
		ctx.Set(gqlbinding.MetadataKeyContext, gctx)

		out, err := rt.Execute(ctx)
		if err != nil {
			t.Fatalf("execute validateArgs: %v", err)
		}
		m := out.(map[string]any)
		if m["id"] != "ok" {
			t.Fatalf("out=%v", out)
		}
	}
}

func TestAdapter_ServeHTTP(t *testing.T) {
	rt, reg := buildRuntimeForTest(t)
	a := New()
	if err := a.Mount(&fakeHost{rt: rt, reg: reg}); err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/graphql", strings.NewReader(`{"query":"query GetUser($tenant: String!, $show: Boolean!) { user(id: \"42\") { ...UserFields displayName @include(if: $show) } echoComposite(id: \"7\") { id field vars selected } __typename } fragment UserFields on User { id name }","variables":{"tenant":"t9","show":true},"operationName":"GetUser"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"user":{"displayName":"alice-display","id":"42","name":"alice"}`) {
		t.Fatalf("unexpected response body: %s", body)
	}
	if !strings.Contains(body, `"echoComposite":{"field":"echoComposite","id":"7","selected":4,"vars":"t9"}`) {
		t.Fatalf("unexpected response body: %s", body)
	}
	if !strings.Contains(body, `"__typename":"Query"`) {
		t.Fatalf("unexpected response body: %s", body)
	}
}

func TestAdapter_BatchAndIntrospection(t *testing.T) {
	rt, reg := buildRuntimeForTest(t)
	a := New().WithPlayground(true)
	if err := a.Mount(&fakeHost{rt: rt, reg: reg}); err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/graphql", strings.NewReader(`[
		{"query":"query { __schema { queryType { name } } }"},
		{"query":"query { user(id: \"9\") { id name displayName } }"}
	]`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `"queryType":{"name":"Query"}`) {
		t.Fatalf("unexpected batch body: %s", body)
	}
	if !strings.Contains(body, `"displayName":"alice-display"`) {
		t.Fatalf("unexpected batch body: %s", body)
	}

	playgroundReq := httptest.NewRequest(http.MethodGet, "http://example.com/playground", nil)
	playgroundRec := httptest.NewRecorder()
	a.ServeHTTP(playgroundRec, playgroundReq)
	if playgroundRec.Code != http.StatusOK || !strings.Contains(playgroundRec.Body.String(), "GraphQL Playground") {
		t.Fatalf("unexpected playground response: %d %s", playgroundRec.Code, playgroundRec.Body.String())
	}
}

func TestCompile_RejectSubscription(t *testing.T) {
	resolver := &testResolver{}
	root := module.CreateModule(nil, nil, nil, []any{resolver}, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)
	reg := registry.New()

	NewAPI(reg).Resolver("User", resolver).
		Subscription("userSub", resolver.User).
		Build()

	if _, err := compiler.Compile(moduleRef, reg, NewPlugin()); err == nil || !strings.Contains(err.Error(), "subscription") {
		t.Fatalf("expected subscription compile error, got %v", err)
	}
}

func TestAdapter_UnboundArgError(t *testing.T) {
	rt, reg := buildRuntimeForTest(t)
	a := New()
	if err := a.Mount(&fakeHost{rt: rt, reg: reg}); err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/graphql", strings.NewReader(`{"query":"query { unbound(first: \"a\", second: \"b\") }"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "ambiguous graphql arg binding") {
		t.Fatalf("unexpected response body: %s", body)
	}
}

func TestAdapter_ValidateArgsError(t *testing.T) {
	rt, reg := buildRuntimeForTest(t)
	a := New()
	if err := a.Mount(&fakeHost{rt: rt, reg: reg}); err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://example.com/graphql", strings.NewReader(`{"query":"query { validateArgs(id: \"x\") { id } }"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "validation failed on ID") {
		t.Fatalf("unexpected response body: %s", body)
	}
}
