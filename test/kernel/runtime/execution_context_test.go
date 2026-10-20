package runtime_test

// 锁定「同一请求内所有 AOP 层共享同一个 ExecutionContext」这一约定。
//
// 优化前 middleware/guard/interceptor 各自构造一份 ExecutionContext（每请求 2~4 次分配），
// 因此它们看到的上下文对象并不相同；现在统一由 Pipeline.Run 构造一次并向下传递。

import (
	"errors"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
	. "github.com/sao-lang/lania-g/kernel/v3/runtime"
)

type ctxProbeCtrl struct{}

func (c *ctxProbeCtrl) Fail() (any, error) { return nil, errors.New("boom") }

func TestPipeline_AOPLayersShareOneExecutionContext(t *testing.T) {
	var (
		middlewareCtx *aop.ExecutionContext
		guardCtx      *aop.ExecutionContext
		interceptCtx  *aop.ExecutionContext
		filterCtx     *aop.ExecutionContext
	)

	p := NewPipeline()
	p.UseGlobalMiddlewares(aop.MiddlewareFunc(func(ctx *aop.ExecutionContext, next func() error) error {
		middlewareCtx = ctx
		return next()
	}))
	p.UseGlobalGuards(aop.GuardFunc(func(ctx *aop.ExecutionContext) (bool, error) {
		guardCtx = ctx
		return true, nil
	}))
	p.UseGlobalInterceptors(aop.InterceptorFunc(func(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
		interceptCtx = ctx
		return next.Handle()
	}))
	p.UseGlobalFilters(aop.ExceptionFilterFunc(func(exception any, ctx *aop.ExecutionContext) error {
		filterCtx = ctx
		return nil
	}))

	handler, err := NewHandler(&ctxProbeCtrl{}, "Fail")
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	handler.Meta.Protocol = Protocol("http")

	ctx := AcquireHandlerContext(Protocol("http"))
	defer ReleaseHandlerContext(ctx)
	ctx.Request.Method = "GET"
	ctx.Request.Path = "/probe"

	if _, err := p.Run(ctx, handler, nil); err != nil {
		t.Fatalf("run: %v", err)
	}

	for name, got := range map[string]*aop.ExecutionContext{
		"middleware":  middlewareCtx,
		"guard":       guardCtx,
		"interceptor": interceptCtx,
		"filter":      filterCtx,
	} {
		if got == nil {
			t.Fatalf("%s did not run", name)
		}
		if got != middlewareCtx {
			t.Fatalf("%s 看到的 ExecutionContext 与 middleware 不同", name)
		}
	}
}
