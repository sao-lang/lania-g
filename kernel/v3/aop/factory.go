// factory.go 把「延迟构造器」适配为可执行的 AOP 接口。
//
// 有了这些适配器，各协议 DSL 只需要一套类型化的 Use* 方法：
// 构造器用对应的 Factory 包一层即可，不必为每个协议再开一套 Use*Factories。
package aop

// GuardFactory 把延迟构造器适配为 Guard。
//
// 构造器在**首次调用时**实例化并缓存，之后复用同一个实例。
func GuardFactory(ctor GuardConstructor) Guard {
	return &lazyGuard{ctor: ctor}
}

// InterceptorFactory 把延迟构造器适配为 Interceptor。
func InterceptorFactory(ctor InterceptorConstructor) Interceptor {
	return &lazyInterceptor{ctor: ctor}
}

// MiddlewareFactory 把延迟构造器适配为 Middleware。
func MiddlewareFactory(ctor MiddlewareConstructor) Middleware {
	return &lazyMiddleware{ctor: ctor}
}

// PipeFactory 把延迟构造器适配为 Pipe。
func PipeFactory(ctor PipeConstructor) Pipe {
	return &lazyPipe{ctor: ctor}
}

// FilterFactory 把延迟构造器适配为 ExceptionFilter。
func FilterFactory(ctor ExceptionFilterConstructor) ExceptionFilter {
	return &lazyFilter{ctor: ctor}
}

type lazyGuard struct {
	ctor     GuardConstructor
	instance Guard
}

func (l *lazyGuard) CanActivate(ctx *ExecutionContext) (bool, error) {
	if l.instance == nil && l.ctor != nil {
		l.instance = l.ctor()
	}
	if l.instance == nil {
		return true, nil
	}
	return l.instance.CanActivate(ctx)
}

type lazyInterceptor struct {
	ctor     InterceptorConstructor
	instance Interceptor
}

func (l *lazyInterceptor) Intercept(ctx *ExecutionContext, next CallHandler) (interface{}, error) {
	if l.instance == nil && l.ctor != nil {
		l.instance = l.ctor()
	}
	if l.instance == nil {
		return next.Handle()
	}
	return l.instance.Intercept(ctx, next)
}

type lazyMiddleware struct {
	ctor     MiddlewareConstructor
	instance Middleware
}

func (l *lazyMiddleware) Use(ctx *ExecutionContext, next func() error) error {
	if l.instance == nil && l.ctor != nil {
		l.instance = l.ctor()
	}
	if l.instance == nil {
		return next()
	}
	return l.instance.Use(ctx, next)
}

type lazyPipe struct {
	ctor     PipeConstructor
	instance Pipe
}

func (l *lazyPipe) Transform(value interface{}, metadata interface{}) (interface{}, error) {
	if l.instance == nil && l.ctor != nil {
		l.instance = l.ctor()
	}
	if l.instance == nil {
		return value, nil
	}
	return l.instance.Transform(value, metadata)
}

type lazyFilter struct {
	ctor     ExceptionFilterConstructor
	instance ExceptionFilter
}

func (l *lazyFilter) Catch(exception interface{}, ctx *ExecutionContext) error {
	if l.instance == nil && l.ctor != nil {
		l.instance = l.ctor()
	}
	if l.instance == nil {
		return nil
	}
	return l.instance.Catch(exception, ctx)
}
