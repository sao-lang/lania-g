package terminus

import (
	stdctx "context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

func TestForRoot_RegistersHealthServiceFactoryAndConfig(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRoot(Config{Version: "1.0.0", ReleaseID: "r1"})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(interface{ SetRegistry(*registry.Registry) }).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	serviceToken := reflect.TypeFor[*HealthService]()
	factoryToken := reflect.TypeFor[Factory]()

	svcAny, err := mod.Container().Get(serviceToken)
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	svc := svcAny.(*HealthService)
	svc.AddIndicator(NewRedisIndicatorFunc("redis", func() error { return nil }))
	svc.AddIndicator(NewRedisIndicatorFunc("broken", func() error { return errors.New("boom") }))
	result := svc.Check()
	if result.Version != "1.0.0" || result.ReleaseID != "r1" {
		t.Fatalf("result=%+v", result)
	}
	if result.Status != StatusFail {
		t.Fatalf("status=%s", result.Status)
	}

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}
	ctx := runtime.NewHandlerContext("test")
	ctx.WithContext(stdctx.Background())
	ctx.Container = mod.Container().NewChild()

	if _, err := br.Resolve(ctx, nil, serviceToken, 0); err != nil {
		t.Fatalf("resolve service: %v", err)
	}
	factoryAny, err := br.Resolve(ctx, nil, factoryToken, 1)
	if err != nil {
		t.Fatalf("resolve factory: %v", err)
	}
	derived, err := factoryAny.(Factory).New(Config{Version: "2.0.0"})
	if err != nil || derived == nil {
		t.Fatalf("derived service: %v", err)
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
	if got := registry.Global().SnapshotFallbackUsage()["integrations/terminus.ForRootCompat"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}
