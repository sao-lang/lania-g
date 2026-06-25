package config

import (
	stdctx "context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	coredi "github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

func TestForRoot_RegistersLoaderFactoryAndConfig(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	dir := t.TempDir()
	path := filepath.Join(dir, "app.json")
	if err := os.WriteFile(path, []byte(`{"app":{"name":"demo"}}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("APP__UNUSED", "")
	t.Setenv("DEMO_APP_PORT", "8080")

	mod, err := ForRoot(Config{
		Files:     []string{path},
		EnvPrefix: "DEMO",
		Data: map[string]interface{}{
			"app": map[string]interface{}{"env": "test"},
		},
	})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(interface{ SetRegistry(*registry.Registry) }).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	loader, err := coredi.GetByType[*Loader](mod.Container())
	if err != nil {
		t.Fatalf("get loader: %v", err)
	}
	if got := loader.GetString("app.name"); got != "demo" {
		t.Fatalf("app.name=%s", got)
	}
	if got := loader.GetString("app.env"); got != "test" {
		t.Fatalf("app.env=%s", got)
	}
	if got := loader.GetString("app.port"); got != "8080" {
		t.Fatalf("app.port=%s", got)
	}

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}
	ctx := runtime.NewHandlerContext("test")
	ctx.WithContext(stdctx.Background())
	ctx.Container = mod.Container().NewChild()

	value, err := br.Resolve(ctx, nil, reflect.TypeFor[*Loader](), 0)
	if err != nil {
		t.Fatalf("resolve loader: %v", err)
	}
	if value.(*Loader) != loader {
		t.Fatalf("resolved loader mismatch")
	}

	factoryToken := reflect.TypeFor[Factory]()
	factoryValue, err := br.Resolve(ctx, nil, factoryToken, 1)
	if err != nil {
		t.Fatalf("resolve factory: %v", err)
	}
	factory := factoryValue.(Factory)
	derived, err := factory.New(Config{Data: map[string]interface{}{"x": "y"}})
	if err != nil {
		t.Fatalf("new loader: %v", err)
	}
	if derived.GetString("x") != "y" {
		t.Fatalf("derived x=%s", derived.GetString("x"))
	}
}

type appSection struct {
	Name string `json:"name"`
	Port string `json:"port"`
}

func TestBindings_ResolveSectionAndValue(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRoot(Config{
		Data: map[string]interface{}{
			"app": map[string]interface{}{
				"name": "demo",
				"port": "8080",
			},
		},
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

	sectionType := reflect.TypeOf(Section[appSection]{})
	sectionHandler := &runtime.Handler{
		Meta: &runtime.HandlerMeta{
			ParamPlans: []runtime.ParamPlan{{Index: 0, Type: sectionType, BindingName: "app"}},
		},
	}
	sectionAny, err := br.Resolve(ctx, sectionHandler, sectionType, 0)
	if err != nil {
		t.Fatalf("resolve section: %v", err)
	}
	section := sectionAny.(Section[appSection])
	if section.Value.Name != "demo" || section.Value.Port != "8080" {
		t.Fatalf("section=%+v", section.Value)
	}

	valueType := reflect.TypeOf(Value[string]{})
	valueHandler := &runtime.Handler{
		Meta: &runtime.HandlerMeta{
			ParamPlans: []runtime.ParamPlan{{Index: 0, Type: valueType, BindingName: "app.name"}},
		},
	}
	valueAny, err := br.Resolve(ctx, valueHandler, valueType, 0)
	if err != nil {
		t.Fatalf("resolve value: %v", err)
	}
	value := valueAny.(Value[string])
	if value.Value != "demo" {
		t.Fatalf("value=%s", value.Value)
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
	if got := registry.Global().SnapshotFallbackUsage()["integrations/config.ForRootCompat"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}
