package application

import (
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

type contextAwareModule struct {
	*module.BaseModule
	got *module.Context
}

func newContextAwareModule() *contextAwareModule {
	return &contextAwareModule{BaseModule: module.New(module.Metadata{})}
}

func (m *contextAwareModule) SetModuleContext(ctx *module.Context) {
	m.BaseModule.SetModuleContext(ctx)
	m.got = ctx
}

func TestNew_CreatesInstanceRegistryAndModuleContext(t *testing.T) {
	root := newContextAwareModule()

	app, err := New(root, Options{})
	if err != nil {
		t.Fatalf("new application: %v", err)
	}
	if app.Registry() == nil {
		t.Fatalf("application registry is nil")
	}
	if app.Registry() == registry.Global() {
		t.Fatalf("application must not use the global registry")
	}
	if root.got == nil {
		t.Fatalf("module context was not injected")
	}
	if root.got.Registry() != app.Registry() {
		t.Fatalf("module context registry mismatch")
	}
}

func TestNew_RequiresRootModule(t *testing.T) {
	if _, err := New(nil, Options{}); err == nil {
		t.Fatalf("expected error for nil root module")
	}
}

func TestNew_MergesModuleAOP(t *testing.T) {
	root := module.New(module.Metadata{
		AOP: module.AOP{Guards: []aop.GuardFunc{func(*aop.ExecutionContext) (bool, error) { return true, nil }}},
	})
	app, err := New(root, Options{})
	if err != nil {
		t.Fatalf("new application: %v", err)
	}
	if got := app.Registry().GetGlobalAOP(); len(got.Guards) != 1 {
		t.Fatalf("global guards = %d, want 1", len(got.Guards))
	}
}
