package application

import (
	"strings"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

type registryAwareModule struct {
	*module.BaseModule
	got *registry.Registry
}

func newRegistryAwareModule() *registryAwareModule {
	return &registryAwareModule{BaseModule: module.NewBaseModule(&module.ModuleMetadata{})}
}

func (m *registryAwareModule) SetRegistry(reg *registry.Registry) { m.got = reg }

func TestNewWithOptions_InjectsRegistryIntoModules(t *testing.T) {
	reg := NewRegistry()
	root := newRegistryAwareModule()

	app, err := NewWithOptions(root, Options{Registry: reg})
	if err != nil {
		t.Fatalf("new with options: %v", err)
	}
	if app.Registry() != reg {
		t.Fatalf("application registry mismatch")
	}
	if root.got != reg {
		t.Fatalf("module registry mismatch")
	}
}

func TestNewRegistry_ReturnsInstanceRegistry(t *testing.T) {
	registry.ResetGlobal()
	t.Cleanup(registry.ResetGlobal)

	reg := NewRegistry()
	if reg == nil {
		t.Fatalf("registry is nil")
	}
	if reg == registry.Global() {
		t.Fatalf("new registry should not reuse global registry")
	}
}

func TestNewCompat_UsesGlobalRegistry(t *testing.T) {
	registry.ResetGlobal()
	t.Cleanup(registry.ResetGlobal)

	root := newRegistryAwareModule()
	app, err := NewCompat(root)
	if err != nil {
		t.Fatalf("new compat: %v", err)
	}
	if app.Registry() != registry.Global() {
		t.Fatalf("application registry mismatch")
	}
	if root.got != registry.Global() {
		t.Fatalf("module registry mismatch")
	}
}

func TestNewWithOptions_RequiresExplicitRegistry(t *testing.T) {
	root := newRegistryAwareModule()

	_, err := NewWithOptions(root, Options{})
	if err == nil {
		t.Fatalf("expected missing registry error")
	}
	if !strings.Contains(err.Error(), "requires an explicit registry") {
		t.Fatalf("unexpected error: %v", err)
	}
}
