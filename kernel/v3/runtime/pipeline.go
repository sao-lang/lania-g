// pipeline.go 实现 runtime 的 AOP 执行管道。
//
// 这一层并不关心具体协议，只关心：
// - 当前 handler 有哪些 middlewares/guards/interceptors/pipes/filters
// - 它们按什么顺序组合
// - 出错后由谁兜底转换/消费
package runtime

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
)

// Pipeline 是 runtime 的 AOP 执行管道。
//
// 它负责统一管理全局 middlewares、guards、interceptors、pipes、filters，
// 并在每次 handler 执行时按约定顺序把它们串起来。
type Pipeline struct {
	globalMiddlewares  []aop.MiddlewareFunc
	globalGuards       []aop.GuardFunc
	globalInterceptors []aop.InterceptorFunc
	globalPipes        []aop.PipeFunc
	globalFilters      []aop.ExceptionFilterFunc
}

// NewPipeline 创建一个新的执行管道。
// 默认所有全局 AOP 列表都是空切片，后续按需追加。
func NewPipeline() *Pipeline {
	return &Pipeline{
		globalMiddlewares:  make([]aop.MiddlewareFunc, 0),
		globalGuards:       make([]aop.GuardFunc, 0),
		globalInterceptors: make([]aop.InterceptorFunc, 0),
		globalPipes:        make([]aop.PipeFunc, 0),
		globalFilters:      make([]aop.ExceptionFilterFunc, 0),
	}
}

// UseGlobalMiddlewares 注册全局中间件。
func (p *Pipeline) UseGlobalMiddlewares(middlewares ...aop.MiddlewareFunc) *Pipeline {
	p.globalMiddlewares = append(p.globalMiddlewares, middlewares...)
	return p
}

// UseGlobalGuards 注册全局守卫。
func (p *Pipeline) UseGlobalGuards(guards ...aop.GuardFunc) *Pipeline {
	p.globalGuards = append(p.globalGuards, guards...)
	return p
}

// UseGlobalInterceptors 注册全局拦截器。
func (p *Pipeline) UseGlobalInterceptors(interceptors ...aop.InterceptorFunc) *Pipeline {
	p.globalInterceptors = append(p.globalInterceptors, interceptors...)
	return p
}

// UseGlobalPipes 注册全局管道。
func (p *Pipeline) UseGlobalPipes(pipes ...aop.PipeFunc) *Pipeline {
	p.globalPipes = append(p.globalPipes, pipes...)
	return p
}

// UseGlobalFilters 注册全局异常过滤器。
func (p *Pipeline) UseGlobalFilters(filters ...aop.ExceptionFilterFunc) *Pipeline {
	p.globalFilters = append(p.globalFilters, filters...)
	return p
}

// Run 执行管道。
//
// 执行顺序（从外到内）：
// 1) Middlewares: 允许包裹整个执行过程，决定是否继续调用 next()
// 2) Guards: 逐个判定是否允许执行；返回 false 视为拒绝
// 3) Interceptors: 允许包裹 handler 调用，做观测/缓存/重试等
// 4) Pipes: 在 handler 调用前对入参做 Transform；调用后对返回值做 Transform
// 5) ExceptionFilters: 当执行链路返回 error 时，按顺序处理/转换 error
//
// 注意：
// - 如果 handler.Meta.CompiledAOP != nil，表示来自编译期产物，直接使用 CompiledAOP 上的 AOP 列表，
//   不再与全局/运行期声明的 AOP 合并（避免重复/顺序歧义）。
// - filter 约定：返回 nil 表示该错误已被“消费”（例如写入响应），上层不再继续抛出。
func (p *Pipeline) Run(ctx *HandlerContext, handler *Handler, args []reflect.Value) (result interface{}, err error) {
	// 单次请求中的 panic 不应影响整个进程，因此这里统一兜底恢复。
	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
			err = fmt.Errorf("%w: %v", ErrExecutionFailed, recovered)
		}
	}()

	var middlewares []aop.MiddlewareFunc
	var guards []aop.GuardFunc
	var interceptors []aop.InterceptorFunc
	var pipes []aop.PipeFunc
	var filters []aop.ExceptionFilterFunc

	if handler.Meta.CompiledAOP != nil {
		// 编译期 AOP 计划一旦存在，就视为“最终权威结果”，
		// 运行时不再把全局和局部列表重新拼一遍，避免重复或顺序歧义。
		middlewares = handler.Meta.CompiledAOP.Middlewares
		guards = handler.Meta.CompiledAOP.Guards
		interceptors = handler.Meta.CompiledAOP.Interceptors
		pipes = handler.Meta.CompiledAOP.Pipes
		filters = handler.Meta.CompiledAOP.Filters
	} else {
		middlewares = p.collectMiddlewares(handler)
		guards = p.collectGuards(handler)
		interceptors = p.collectInterceptors(handler)
		pipes = p.collectPipes(handler)
		filters = p.collectFilters(handler)
	}

	execCtx := p.newExecutionContext(ctx, handler)

	middlewareHandler := p.buildMiddlewareChain(middlewares, func() error {
		// Guards 在进入 handler 之前执行；任意 guard 返回 error 或 false 都会中断。
		for _, guard := range guards {
			canActivate, guardErr := guard(execCtx)
			if guardErr != nil {
				return guardErr
			}
			if !canActivate {
				return ErrGuardRejected
			}
		}

		interceptorHandler := func() (any, error) {
			// Pipes(input): 每个 pipe 可以对所有入参做 Transform。
			for _, pipe := range pipes {
				for i, arg := range args {
					metadata := &aop.ArgumentMetadata{
						Type:  handler.Meta.ParamTypes[i],
						Data:  "input",
					}
					transformed, pipeErr := pipe.Transform(arg.Interface(), metadata)
					if pipeErr != nil {
						return nil, pipeErr
					}
					args[i] = reflect.ValueOf(transformed)
				}
			}

			results, invokeErr := handler.Invoke(ctx, args)
			if invokeErr != nil {
				return nil, invokeErr
			}

			// 约定：如果最后一个返回值是 error，则将其视为执行错误；
			// 否则将第一个返回值作为业务结果。
			// 这和大多数 Go handler 的 `(result, error)` 约定保持一致。
			var localResult any
			if len(results) > 0 {
				lastResult := results[len(results)-1]
				if lastResult.Type().Implements(reflect.TypeFor[error]()) {
					if !lastResult.IsNil() {
						return nil, lastResult.Interface().(error)
					}
					if len(results) > 1 {
						localResult = results[0].Interface()
					}
				} else if len(results) > 0 {
					localResult = results[0].Interface()
				}
			}

			// Pipes(output): 对返回值做 Transform（只针对第一个返回值的类型元信息）。
			for _, pipe := range pipes {
				if len(handler.Meta.ReturnTypes) == 0 {
					break
				}
				metadata := &aop.ArgumentMetadata{
					Type: handler.Meta.ReturnTypes[0],
					Data: "output",
				}
				transformed, pipeErr := pipe.Transform(localResult, metadata)
				if pipeErr != nil {
					return nil, pipeErr
				}
				localResult = transformed
			}

			return localResult, nil
		}

		// Interceptors 包裹 interceptorHandler，通常用于 tracing / metrics / cache / retry 等。
		result, err = p.buildInterceptorChain(interceptors, interceptorHandler, ctx)
		return err
	}, ctx, handler)

	err = middlewareHandler()

	if err != nil {
		// Filters 只在发生 error 时执行；filter 可以选择“吞掉错误”（返回 nil）。
		for _, filter := range filters {
			filterErr := filter(err, execCtx)
			if filterErr == nil {
				return nil, nil
			}
			err = filterErr
		}
	}

	return result, err
}

// collectMiddlewares 收集某个 handler 实际生效的 Middlewares 列表。
//
// 规则：全局在前、handler 局部在后（即“越靠后越靠近 handler 本体”）。
func (p *Pipeline) collectMiddlewares(handler *Handler) []aop.MiddlewareFunc {
	all := make([]aop.MiddlewareFunc, 0, len(p.globalMiddlewares)+len(handler.Meta.Middlewares))
	all = append(all, p.globalMiddlewares...)
	all = append(all, handler.Meta.Middlewares...)
	return all
}

// collectGuards 收集某个 handler 实际生效的 Guards 列表。
//
// 规则：全局在前、handler 局部在后。
func (p *Pipeline) collectGuards(handler *Handler) []aop.GuardFunc {
	all := make([]aop.GuardFunc, 0, len(p.globalGuards)+len(handler.Meta.Guards))
	all = append(all, p.globalGuards...)
	all = append(all, handler.Meta.Guards...)
	return all
}

// collectInterceptors 收集某个 handler 实际生效的 Interceptors 列表。
//
// 规则：全局在前、handler 局部在后。
func (p *Pipeline) collectInterceptors(handler *Handler) []aop.InterceptorFunc {
	all := make([]aop.InterceptorFunc, 0, len(p.globalInterceptors)+len(handler.Meta.Interceptors))
	all = append(all, p.globalInterceptors...)
	all = append(all, handler.Meta.Interceptors...)
	return all
}

// collectPipes 收集某个 handler 实际生效的 Pipes 列表。
//
// 规则：全局在前、handler 局部在后。
func (p *Pipeline) collectPipes(handler *Handler) []aop.PipeFunc {
	all := make([]aop.PipeFunc, 0, len(p.globalPipes)+len(handler.Meta.Pipes))
	all = append(all, p.globalPipes...)
	all = append(all, handler.Meta.Pipes...)
	return all
}

// collectFilters 收集某个 handler 实际生效的 ExceptionFilters 列表。
//
// 规则：全局在前、handler 局部在后。
func (p *Pipeline) collectFilters(handler *Handler) []aop.ExceptionFilterFunc {
	all := make([]aop.ExceptionFilterFunc, 0, len(p.globalFilters)+len(handler.Meta.Filters))
	all = append(all, p.globalFilters...)
	all = append(all, handler.Meta.Filters...)
	return all
}

// buildMiddlewareChain 将 middlewares 编译为一个可执行的链式函数（返回 func() error）。
//
// 为什么这样实现：
// - 使用“迭代 + 单个 next 闭包”而不是递归嵌套闭包，减少分配与调用栈深度
// - middleware 约定为：middleware(execCtx, next)；middleware 可选择不调用 next 来中断执行
func (p *Pipeline) buildMiddlewareChain(middlewares []aop.MiddlewareFunc, finalHandler func() error, ctx *HandlerContext, routeHandler *Handler) func() error {
	execCtx := p.newExecutionContext(ctx, routeHandler)
	if len(middlewares) == 0 {
		return finalHandler
	}

	// 这里采用迭代式链路执行，只为每次请求分配一个 next 闭包，
	// 避免为每个 middleware 都额外创建一层闭包。
	return func() error {
		index := -1
		var next func() error
		next = func() error {
			index++
			if index < len(middlewares) {
				return middlewares[index](execCtx, next)
			}
			return finalHandler()
		}
		return next()
	}
}

// buildInterceptorChain 将 interceptors 编译为一个可执行的链式调用。
//
// 拦截器与 middleware 的区别：
// - middleware 关注“是否继续执行链路”（返回 error）
// - interceptor 关注“包裹 handler 调用并处理结果”（返回 (result, error)）
//
// 这里同样使用迭代方式，避免递归/深层闭包带来的额外开销。
func (p *Pipeline) buildInterceptorChain(interceptors []aop.InterceptorFunc, finalHandler func() (interface{}, error), ctx *HandlerContext) (interface{}, error) {
	if len(interceptors) == 0 {
		return finalHandler()
	}

	// 这里用迭代式状态机维护拦截器链，避免递归链带来的额外闭包和栈深。
	type chainHandler struct {
		p            *Pipeline
		ctx          *HandlerContext
		interceptors []aop.InterceptorFunc
		index        int
		final        func() (interface{}, error)
	}
	var h chainHandler
	h.p = p
	h.ctx = ctx
	h.interceptors = interceptors
	h.index = -1
	h.final = finalHandler

	var next aop.CallHandler
	next = aop.NewCallHandler(func() (interface{}, error) {
		h.index++
		if h.index >= len(h.interceptors) {
			return h.final()
		}
		it := h.interceptors[h.index]
		return it(h.p.newExecutionContext(h.ctx, nil), next)
	})
	return next.Handle()
}

// newExecutionContext 将 runtime.HandlerContext 转换/映射为 aop.ExecutionContext。
//
// ExecutionContext 是 AOP 层统一使用的上下文结构，包含：
// - 请求/响应抽象（Request/Response）
// - 当前 handler 信息（routeKey/protocol/moduleKey 等）
// - DI container（用于 interceptor/middleware 内部 resolve 依赖）
//
// 注意：
// - handler 可能为 nil（例如 interceptor 链构造阶段）；此时 class/handler 相关字段会为空
// - class 的选择优先级：handler.Instance（运行期注册） > handler.ReceiverToken（编译期 token）
func (p *Pipeline) newExecutionContext(ctx *HandlerContext, handler *Handler) *aop.ExecutionContext {
	var class interface{}
	if handler != nil {
		if handler.Instance != nil {
			class = handler.Instance
		} else {
			class = handler.ReceiverToken
		}
	}
	request := interface{}(ctx.Request)
	response := interface{}(ctx.Response)
	execCtx := aop.NewExecutionContext(ctx.GetContext(), ctx, handler, class)
	execCtx.Request = request
	execCtx.Response = response
	execCtx.Container = ctx.Container
	execCtx.Protocol = string(ctx.Protocol)
	execCtx.RouteKey = ctx.RouteKey
	if moduleKey, ok := ctx.Get("kernel.moduleKey"); ok {
		if value, ok := moduleKey.(string); ok {
			execCtx.ModuleKey = value
		}
	}
	return execCtx
}
