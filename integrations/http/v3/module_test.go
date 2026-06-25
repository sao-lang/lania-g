package http

import (
	stdctx "context"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	coredi "github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

func TestForRoot_RegistersHTTPClientFactoryAndConfig(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		if r.Header.Get("X-Test") != "1" {
			w.WriteHeader(stdhttp.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()

	mod, err := ForRoot(Config{
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
		DefaultHeaders: map[string]string{
			"X-Test": "1",
		},
	})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(interface{ SetRegistry(*registry.Registry) }).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	client, err := coredi.GetByType[*Client](mod.Container())
	if err != nil {
		t.Fatalf("get client: %v", err)
	}
	resp, err := client.Get("/health")
	if err != nil {
		t.Fatalf("http get: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Fatalf("status=%d", resp.StatusCode())
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
	derived, err := factory.New(Config{BaseURL: server.URL, DefaultHeaders: map[string]string{"X-Test": "1"}})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	resp, err = derived.Get("/health")
	if err != nil {
		t.Fatalf("derived get: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Fatalf("derived status=%d", resp.StatusCode())
	}
}

type adminHTTPClient struct{}

func (adminHTTPClient) HTTPClientName() string { return "admin" }

func TestBindings_ResolveNamedClientRef(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		_, _ = io.WriteString(w, `ok`)
	}))
	defer server.Close()

	mod, err := ForRoot(Config{
		Name:    "admin",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(interface{ SetRegistry(*registry.Registry) }).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}
	ctx := runtime.NewHandlerContext("test")
	ctx.WithContext(stdctx.Background())
	ctx.Container = mod.Container().NewChild()

	refType := reflect.TypeOf(ClientRef[adminHTTPClient]{})
	handler := &runtime.Handler{
		Meta: &runtime.HandlerMeta{
			ParamPlans: []runtime.ParamPlan{{Index: 0, Type: refType}},
		},
	}
	value, err := br.Resolve(ctx, handler, refType, 0)
	if err != nil {
		t.Fatalf("resolve named client ref: %v", err)
	}
	ref := value.(ClientRef[adminHTTPClient])
	resp, err := ref.Get("/")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Fatalf("status=%d", resp.StatusCode())
	}
}

func TestForRoot_InitRequiresExplicitRegistry(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRoot(Config{})
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

	mod, err := ForRootCompat(Config{})
	if err != nil {
		t.Fatalf("for root compat: %v", err)
	}
	if err := mod.Init(); err != nil {
		t.Fatalf("init compat module: %v", err)
	}
	if got := registry.Global().SnapshotFallbackUsage()["integrations/http.ForRootCompat"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}
