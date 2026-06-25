// middleware.go 实现 resilience 集成的运行时中间件与上下文透传逻辑。
package resilience

import (
	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// Middleware 创建一个只负责限流前置检查的中间件。
func Middleware(service *Service) aop.MiddlewareFunc {
	return func(execCtx *aop.ExecutionContext, next func() error) error {
		hc, ok := execCtx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil || service == nil {
			return next()
		}
		if err := service.allowRate(hc); err != nil {
			return err
		}
		return next()
	}
}

// Interceptor 创建一个集成重试、熔断和幂等回放的拦截器。
func Interceptor(service *Service) aop.InterceptorFunc {
	return func(execCtx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
		hc, _ := execCtx.HandlerContext.(*runtime.HandlerContext)
		if done, value, err := service.tryReplay(hc); done || err != nil {
			return value, err
		}
		routeKey := routeKeyOf(hc)
		if err := service.guardBreaker(routeKey); err != nil {
			return nil, err
		}
		attempts := 1
		retryConfig := service.getRetryConfig(routeKey)
		if service != nil && retryConfig.Enabled && retryConfig.MaxAttempts > 1 {
			attempts = retryConfig.MaxAttempts
		}
		var lastErr error
		for attempt := 1; attempt <= attempts; attempt++ {
			result, err := service.invoke(execCtx, next, routeKey)
			if err == nil {
				service.onSuccess(routeKey)
				service.storeReplay(hc, result)
				return result, nil
			}
			lastErr = err
			service.onFailure(routeKey)
			if attempt < attempts && retryConfig.Backoff > 0 {
				timeSleep(retryConfig.Backoff)
			}
		}
		return nil, lastErr
	}
}

// Install 把 resilience 的全局 middleware 与 interceptor 安装到目标对象上。
func Install(into interface {
	UseGlobalMiddlewares(...aop.MiddlewareFunc)
	UseGlobalInterceptors(...aop.InterceptorFunc)
}, service *Service) {
	if into == nil || service == nil {
		return
	}
	into.UseGlobalMiddlewares(Middleware(service))
	into.UseGlobalInterceptors(Interceptor(service))
}
