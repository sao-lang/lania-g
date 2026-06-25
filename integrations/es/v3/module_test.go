package es

import (
	stdctx "context"
	"reflect"
	"strings"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

func TestForRoot_RegistersESClientFactoryAndConfig(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	cfg := Config{
		Addresses: []string{"http://localhost:9200"},
		Username:  "elastic",
		Password:  "secret",
	}
	mod, err := ForRoot(cfg)
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(interface{ SetRegistry(*registry.Registry) }).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	clientToken := reflect.TypeFor[*Client]()
	factoryToken := reflect.TypeFor[Factory]()

	clientAny, err := mod.Container().Get(clientToken)
	if err != nil {
		t.Fatalf("get client: %v", err)
	}
	client := clientAny.(*Client)
	if got := client.GetAddress(); got != "http://localhost:9200" {
		t.Fatalf("address=%s", got)
	}

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}
	ctx := runtime.NewHandlerContext("test")
	ctx.WithContext(stdctx.Background())
	ctx.Container = mod.Container().NewChild()

	value, err := br.Resolve(ctx, nil, clientToken, 0)
	if err != nil {
		t.Fatalf("resolve client: %v", err)
	}
	if value.(*Client) != client {
		t.Fatalf("resolved client mismatch")
	}

	factoryAny, err := br.Resolve(ctx, nil, factoryToken, 1)
	if err != nil {
		t.Fatalf("resolve factory: %v", err)
	}
	derived, err := factoryAny.(Factory).New(Config{Addresses: []string{"http://localhost:9201"}})
	if err != nil {
		t.Fatalf("derived client: %v", err)
	}
	if got := derived.GetAddress(); got != "http://localhost:9201" {
		t.Fatalf("derived address=%s", got)
	}
}

func TestForRoot_InitRequiresExplicitRegistry(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRoot(Config{Addresses: []string{"http://localhost:9200"}})
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

	mod, err := ForRootCompat(Config{Addresses: []string{"http://localhost:9200"}})
	if err != nil {
		t.Fatalf("for root compat: %v", err)
	}
	if err := mod.Init(); err != nil {
		t.Fatalf("init compat module: %v", err)
	}
	if got := registry.Global().SnapshotFallbackUsage()["integrations/es.ForRootCompat"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}
