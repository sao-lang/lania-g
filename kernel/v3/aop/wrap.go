// wrap.go 把 AOP 接口切片收敛为函数类型切片。
//
// 运行时（pipeline）与编译期（compiler）都以函数类型执行增强器，
// 这里提供唯一的收敛入口：已经是函数类型的元素直接透传，避免多余闭包与分配。
package aop

// WrapMiddlewares 把接口切片收敛为 MiddlewareFunc 切片。
func WrapMiddlewares(items []Middleware) []MiddlewareFunc {
	if len(items) == 0 {
		return nil
	}
	out := make([]MiddlewareFunc, 0, len(items))
	for _, item := range items {
		if fn, ok := item.(MiddlewareFunc); ok {
			out = append(out, fn)
			continue
		}
		middleware := item
		out = append(out, func(ctx *ExecutionContext, next func() error) error {
			return middleware.Use(ctx, next)
		})
	}
	return out
}

// WrapGuards 把接口切片收敛为 GuardFunc 切片。
func WrapGuards(items []Guard) []GuardFunc {
	if len(items) == 0 {
		return nil
	}
	out := make([]GuardFunc, 0, len(items))
	for _, item := range items {
		if fn, ok := item.(GuardFunc); ok {
			out = append(out, fn)
			continue
		}
		out = append(out, WrapGuard(item))
	}
	return out
}

// WrapInterceptors 把接口切片收敛为 InterceptorFunc 切片。
func WrapInterceptors(items []Interceptor) []InterceptorFunc {
	if len(items) == 0 {
		return nil
	}
	out := make([]InterceptorFunc, 0, len(items))
	for _, item := range items {
		if fn, ok := item.(InterceptorFunc); ok {
			out = append(out, fn)
			continue
		}
		out = append(out, WrapInterceptor(item))
	}
	return out
}

// WrapPipes 把接口切片收敛为 PipeFunc 切片。
func WrapPipes(items []Pipe) []PipeFunc {
	if len(items) == 0 {
		return nil
	}
	out := make([]PipeFunc, 0, len(items))
	for _, item := range items {
		if fn, ok := item.(PipeFunc); ok {
			out = append(out, fn)
			continue
		}
		out = append(out, WrapPipe(item))
	}
	return out
}

// WrapFilters 把接口切片收敛为 ExceptionFilterFunc 切片。
func WrapFilters(items []ExceptionFilter) []ExceptionFilterFunc {
	if len(items) == 0 {
		return nil
	}
	out := make([]ExceptionFilterFunc, 0, len(items))
	for _, item := range items {
		if fn, ok := item.(ExceptionFilterFunc); ok {
			out = append(out, fn)
			continue
		}
		filter := item
		out = append(out, func(exception interface{}, ctx *ExecutionContext) error {
			return filter.Catch(exception, ctx)
		})
	}
	return out
}

// WrapParamPipes 按参数下标收敛参数管道。
func WrapParamPipes(items map[int][]Pipe) map[int][]PipeFunc {
	if len(items) == 0 {
		return nil
	}
	out := make(map[int][]PipeFunc, len(items))
	for index, pipes := range items {
		out[index] = WrapPipes(pipes)
	}
	return out
}
