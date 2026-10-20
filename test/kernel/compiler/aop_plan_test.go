package compiler_test

// 测试位于仓库根目录 test/，通过 dot-import 访问被测包。
import . "github.com/sao-lang/lania-g/kernel/v3/compiler"
import (
	"errors"
	"reflect"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

func tagMiddleware() aop.MiddlewareFunc {
	return func(ctx *aop.ExecutionContext, next func() error) error { return next() }
}

func TestCompileAOPPlanRejectsUnsupportedItem(t *testing.T) {
	_, err := CompileAOPPlan(registry.GlobalAOPRegistration{}, AOPSources{
		Guards: []any{"not-a-guard"},
	})
	if err == nil {
		t.Fatalf("expected compile error for unsupported guard type")
	}
	var unsupported *aop.ErrUnsupported
	if !errors.As(err, &unsupported) {
		t.Fatalf("error = %T, want *aop.ErrUnsupported", err)
	}
	if unsupported.Kind != "guard" {
		t.Fatalf("kind = %q, want guard", unsupported.Kind)
	}
}

func TestCompileAOPPlanRejectsUnsupportedParamPipe(t *testing.T) {
	_, err := CompileAOPPlan(registry.GlobalAOPRegistration{}, AOPSources{
		ParamPipes: map[int][]any{0: {"not-a-pipe"}},
	})
	if err == nil {
		t.Fatalf("expected compile error for unsupported param pipe type")
	}
}

func TestCompileAOPPlanMergesScopedMiddlewaresBetweenGlobalAndLocal(t *testing.T) {
	global := tagMiddleware()
	scoped := tagMiddleware()
	local := tagMiddleware()

	registration := registry.GlobalAOPRegistration{
		Middlewares: []aop.MiddlewareFunc{global},
		ScopedMiddlewares: []aop.ScopedMiddleware{
			{
				Middlewares:   []aop.MiddlewareFunc{scoped},
				IncludeRoutes: []string{"/users/*"},
			},
		},
	}

	plan, err := CompileAOPPlan(registration, AOPSources{
		Middlewares: []any{local},
		RoutePath:   "/users/:id",
		RouteMethod: "GET",
	})
	if err != nil {
		t.Fatalf("compile aop plan: %v", err)
	}
	if len(plan.Middlewares) != 3 {
		t.Fatalf("middlewares = %d, want 3", len(plan.Middlewares))
	}

	want := []uintptr{
		reflect.ValueOf(global).Pointer(),
		reflect.ValueOf(scoped).Pointer(),
		reflect.ValueOf(local).Pointer(),
	}
	for i, fn := range plan.Middlewares {
		if got := reflect.ValueOf(fn).Pointer(); got != want[i] {
			t.Fatalf("middleware[%d] is out of order", i)
		}
	}
}

func TestCompileAOPPlanSkipsScopedMiddlewaresOutsideRange(t *testing.T) {
	registration := registry.GlobalAOPRegistration{
		ScopedMiddlewares: []aop.ScopedMiddleware{
			{
				Middlewares:   []aop.MiddlewareFunc{tagMiddleware()},
				IncludeRoutes: []string{"/users/*"},
			},
		},
	}

	plan, err := CompileAOPPlan(registration, AOPSources{
		RoutePath:   "/orders/:id",
		RouteMethod: "GET",
	})
	if err != nil {
		t.Fatalf("compile aop plan: %v", err)
	}
	if len(plan.Middlewares) != 0 {
		t.Fatalf("middlewares = %d, want 0", len(plan.Middlewares))
	}
}
