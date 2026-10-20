package compiler_test

// 测试位于仓库根目录 test/，通过 dot-import 访问被测包。
import . "github.com/sao-lang/lania-g/kernel/v3/compiler"
import (
	"reflect"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

func tagMiddleware() aop.MiddlewareFunc {
	return func(ctx *aop.ExecutionContext, next func() error) error { return next() }
}

func TestCompileAOPPlanMergesScopedMiddlewaresBetweenGlobalAndLocal(t *testing.T) {
	global := tagMiddleware()
	scoped := tagMiddleware()
	local := tagMiddleware()

	registration := registry.GlobalAOPRegistration{
		Middlewares: []aop.Middleware{global},
		ScopedMiddlewares: []aop.ScopedMiddleware{
			{
				Middlewares:   []aop.MiddlewareFunc{scoped},
				IncludeRoutes: []string{"/users/*"},
			},
		},
	}

	plan := CompileAOPPlan(registration, AOPSources{
		Middlewares: []aop.Middleware{local},
		RoutePath:   "/users/:id",
		RouteMethod: "GET",
	})
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

	plan := CompileAOPPlan(registration, AOPSources{
		RoutePath:   "/orders/:id",
		RouteMethod: "GET",
	})
	if len(plan.Middlewares) != 0 {
		t.Fatalf("middlewares = %d, want 0", len(plan.Middlewares))
	}
}
