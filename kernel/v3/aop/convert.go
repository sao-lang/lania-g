// convert.go 把 DSL 里“任意形态”的 AOP 声明收敛为可执行函数。
//
// Use* / Apply 的参数类型是 any，允许传接口实现、具名函数类型、裸函数或延迟构造器。
// 收敛逻辑集中在这里，并对不支持的类型直接报错：早期实现缺少 default 分支，
// 传错类型会被静默丢弃，最终只表现为“声明了但没生效”，很难排查。
package aop

import "fmt"

const (
	middlewareForms  = "aop.Middleware / aop.MiddlewareFunc / aop.MiddlewareConstructor / func(*aop.ExecutionContext, func() error) error"
	guardForms       = "aop.Guard / aop.GuardFunc / aop.GuardConstructor / func(*aop.ExecutionContext) (bool, error)"
	interceptorForms = "aop.Interceptor / aop.InterceptorFunc / aop.InterceptorConstructor / func(*aop.ExecutionContext, aop.CallHandler) (interface{}, error)"
	pipeForms        = "aop.Pipe / aop.PipeFunc / aop.PipeConstructor / func(interface{}, interface{}) (interface{}, error)"
	filterForms      = "aop.ExceptionFilter / aop.ExceptionFilterFunc / aop.ExceptionFilterConstructor / func(interface{}, *aop.ExecutionContext) error"
)

// ErrUnsupported 描述一次“声明类型无法转换为可执行 AOP”的错误。
type ErrUnsupported struct {
	Kind     string
	Index    int
	Value    any
	Expected string
}

// Error 实现 error 接口。
func (e *ErrUnsupported) Error() string {
	return fmt.Sprintf("aop: unsupported %s at index %d: %T (supported: %s)", e.Kind, e.Index, e.Value, e.Expected)
}

func unsupported(kind string, index int, item any, forms string) error {
	return &ErrUnsupported{Kind: kind, Index: index, Value: item, Expected: forms}
}

// CoerceMiddlewares 把 middleware 声明列表转换为 MiddlewareFunc 列表。
func CoerceMiddlewares(items []any) ([]MiddlewareFunc, error) {
	out := make([]MiddlewareFunc, 0, len(items))
	for index, item := range items {
		switch v := item.(type) {
		case MiddlewareFunc:
			out = append(out, v)
		case Middleware:
			mw := v
			out = append(out, func(ctx *ExecutionContext, next func() error) error { return mw.Use(ctx, next) })
		case MiddlewareConstructor:
			// 构造器在编译期实例化一次，之后被该路由的所有请求复用。
			if mw := v(); mw != nil {
				out = append(out, func(ctx *ExecutionContext, next func() error) error { return mw.Use(ctx, next) })
			}
		case func(*ExecutionContext, func() error) error:
			out = append(out, MiddlewareFunc(v))
		default:
			return nil, unsupported("middleware", index, item, middlewareForms)
		}
	}
	return out, nil
}

// CoerceGuards 把 guard 声明列表转换为 GuardFunc 列表。
func CoerceGuards(items []any) ([]GuardFunc, error) {
	out := make([]GuardFunc, 0, len(items))
	for index, item := range items {
		switch v := item.(type) {
		case GuardFunc:
			out = append(out, v)
		case Guard:
			out = append(out, WrapGuard(v))
		case GuardConstructor:
			if g := v(); g != nil {
				out = append(out, WrapGuard(g))
			}
		case func(*ExecutionContext) (bool, error):
			out = append(out, GuardFunc(v))
		default:
			return nil, unsupported("guard", index, item, guardForms)
		}
	}
	return out, nil
}

// CoerceInterceptors 把 interceptor 声明列表转换为 InterceptorFunc 列表。
func CoerceInterceptors(items []any) ([]InterceptorFunc, error) {
	out := make([]InterceptorFunc, 0, len(items))
	for index, item := range items {
		switch v := item.(type) {
		case InterceptorFunc:
			out = append(out, v)
		case Interceptor:
			out = append(out, WrapInterceptor(v))
		case InterceptorConstructor:
			if it := v(); it != nil {
				out = append(out, WrapInterceptor(it))
			}
		case func(*ExecutionContext, CallHandler) (interface{}, error):
			out = append(out, InterceptorFunc(v))
		default:
			return nil, unsupported("interceptor", index, item, interceptorForms)
		}
	}
	return out, nil
}

// CoercePipes 把 pipe 声明列表转换为 PipeFunc 列表。
func CoercePipes(items []any) ([]PipeFunc, error) {
	out := make([]PipeFunc, 0, len(items))
	for index, item := range items {
		switch v := item.(type) {
		case PipeFunc:
			out = append(out, v)
		case Pipe:
			out = append(out, WrapPipe(v))
		case PipeConstructor:
			if p := v(); p != nil {
				out = append(out, WrapPipe(p))
			}
		case func(interface{}, interface{}) (interface{}, error):
			out = append(out, PipeFunc(v))
		default:
			return nil, unsupported("pipe", index, item, pipeForms)
		}
	}
	return out, nil
}

// CoerceFilters 把 exception filter 声明列表转换为 ExceptionFilterFunc 列表。
func CoerceFilters(items []any) ([]ExceptionFilterFunc, error) {
	out := make([]ExceptionFilterFunc, 0, len(items))
	for index, item := range items {
		switch v := item.(type) {
		case ExceptionFilterFunc:
			out = append(out, v)
		case ExceptionFilter:
			filter := v
			out = append(out, func(exception interface{}, ctx *ExecutionContext) error {
				return filter.Catch(exception, ctx)
			})
		case ExceptionFilterConstructor:
			if filter := v(); filter != nil {
				out = append(out, func(exception interface{}, ctx *ExecutionContext) error {
					return filter.Catch(exception, ctx)
				})
			}
		case func(interface{}, *ExecutionContext) error:
			out = append(out, ExceptionFilterFunc(v))
		default:
			return nil, unsupported("exception filter", index, item, filterForms)
		}
	}
	return out, nil
}

// CoerceParamPipes 把参数级 pipe 声明（按参数索引分组）转换为可执行形式。
func CoerceParamPipes(items map[int][]any) (map[int][]PipeFunc, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make(map[int][]PipeFunc, len(items))
	for index, defs := range items {
		converted, err := CoercePipes(defs)
		if err != nil {
			return nil, fmt.Errorf("parameter %d: %w", index, err)
		}
		out[index] = converted
	}
	return out, nil
}
