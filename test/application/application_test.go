package application_test

// 测试位于仓库根目录 test/，通过 dot-import 访问被测包。
import . "github.com/sao-lang/lania-g/application/v3"
import (
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/module"
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
		AOP: module.AOP{Guards: []aop.Guard{aop.GuardFunc(func(*aop.ExecutionContext) (bool, error) { return true, nil })}},
	})
	app, err := New(root, Options{})
	if err != nil {
		t.Fatalf("new application: %v", err)
	}
	if got := app.Registry().GetGlobalAOP(); len(got.Guards) != 1 {
		t.Fatalf("global guards = %d, want 1", len(got.Guards))
	}
}

type scopedMiddlewareModule struct {
	*module.BaseModule
}

func newScopedMiddlewareModule() *scopedMiddlewareModule {
	return &scopedMiddlewareModule{BaseModule: module.New(module.Metadata{})}
}

func (m *scopedMiddlewareModule) Configure(consumer aop.MiddlewareConsumer) {
	consumer.Apply(aop.MiddlewareFunc(func(ctx *aop.ExecutionContext, next func() error) error {
		return next()
	})).ForRoutes("/users/*")
}

func TestNew_CollectsScopedMiddlewares(t *testing.T) {
	root := module.New(module.Metadata{Imports: []module.Module{newScopedMiddlewareModule()}})

	app, err := New(root, Options{})
	if err != nil {
		t.Fatalf("new application: %v", err)
	}

	scoped := app.Registry().GetGlobalAOP().ScopedMiddlewares
	if len(scoped) != 1 {
		t.Fatalf("scoped middlewares = %d, want 1", len(scoped))
	}
	if !scoped[0].ShouldApply("GET", "/users/42") {
		t.Fatalf("scoped middleware should apply to /users/42")
	}
	if scoped[0].ShouldApply("GET", "/orders/42") {
		t.Fatalf("scoped middleware should not apply to /orders/42")
	}
}

