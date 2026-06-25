// runtime.go 定义 Runtime 这个对外总入口，以及它对 Router/Pipeline/Executor 的组合关系。
//
// 上层 adapter/application 基本只会直接接触 Runtime，
// 而具体“怎么匹配、怎么解析参数、怎么跑 AOP”则分别委托给内部三个组件。
package runtime

import (
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
)

// Runtime 是运行时核心的轻量门面。
// 它本身不承载复杂逻辑，重点在于把 router/pipeline/executor 组织成一套稳定 API。
type Runtime struct {
	router   *Router
	pipeline *Pipeline
	executor *Executor
}

// NewRuntime 创建新的运行时实例。
//
// Runtime 由三部分组成：
// - Router：负责把 (protocol, method, path) 映射到具体 Handler（支持精确匹配 + 协议自定义 matcher）
// - Pipeline：负责执行 AOP 链路（middlewares/guards/interceptors/pipes/filters）
// - Executor：负责“路由匹配 + 参数解析 + 调用 Pipeline/Handler”，是每次请求的统一入口
func NewRuntime() *Runtime {
	router := NewRouter()
	pipeline := NewPipeline()
	executor := NewExecutor(router, pipeline)

	return &Runtime{
		router:   router,
		pipeline: pipeline,
		executor: executor,
	}
}

// UseGlobalMiddleware 注册全局中间件。
//
// 全局中间件会参与每一次 handler 执行（除非该 handler 使用了编译期 AOP 计划，见 Pipeline.Run 注释）。
func (r *Runtime) UseGlobalMiddleware(middlewares ...aop.MiddlewareFunc) *Runtime {
	r.pipeline.UseGlobalMiddlewares(middlewares...)
	return r
}

// UseGlobalGuards 注册全局守卫。
//
// 守卫通常用于鉴权/限流/开关控制等：当任意 guard 返回 (false, nil) 时，会中断执行并返回 ErrGuardRejected。
func (r *Runtime) UseGlobalGuards(guards ...aop.GuardFunc) *Runtime {
	r.pipeline.UseGlobalGuards(guards...)
	return r
}

// UseGlobalInterceptors 注册全局拦截器。
//
// 拦截器用于包裹 handler 调用，适合做 tracing、metrics、缓存、重试等横切能力。
func (r *Runtime) UseGlobalInterceptors(interceptors ...aop.InterceptorFunc) *Runtime {
	r.pipeline.UseGlobalInterceptors(interceptors...)
	return r
}

// UseGlobalPipes 注册全局 Pipe（管道）。
//
// Pipe 既可用于入参 Transform，也可用于返回值 Transform（见 Pipeline.Run）。
func (r *Runtime) UseGlobalPipes(pipes ...aop.PipeFunc) *Runtime {
	r.pipeline.UseGlobalPipes(pipes...)
	return r
}

// UseGlobalFilters 注册全局异常过滤器。
//
// 当执行链路产生 error 时，filters 会按顺序处理/包装该错误；返回 nil 表示错误已被消费。
func (r *Runtime) UseGlobalFilters(filters ...aop.ExceptionFilterFunc) *Runtime {
	r.pipeline.UseGlobalFilters(filters...)
	return r
}

// Register 注册处理器构建器（通常由协议 adapter 提供）。
//
// builder 会把协议下的路由/handler 映射注册到 Router 中；不同协议只需实现 HandlerBuilder 即可接入 runtime。
func (r *Runtime) Register(builder HandlerBuilder) error {
	return builder.Register(r.router)
}

// Execute 执行一次请求，是运行时对外的统一入口。
//
// 实际执行由 Executor 完成：匹配路由、解析参数、运行 AOP 管道并调用 handler。
func (r *Runtime) Execute(ctx *HandlerContext) (interface{}, error) {
	return r.executor.Execute(ctx)
}

// SetRouter 替换当前 Runtime 使用的 Router。
//
// 使用场景：
// - 单测/热更新时需要替换整套路由表
// - 安装编译期产物后需要切换到新的 Router 实例
//
// 注意：会同步更新 Executor 内部持有的 Router；传入 nil 会被忽略。
func (r *Runtime) SetRouter(router *Router) *Runtime {
	if router == nil {
		return r
	}
	r.router = router
	r.executor.SetRouter(router)
	return r
}

// RegisterBinding 注册参数绑定解析器（BindingResolver）。
//
// BindingResolver 用于把 handler 入参类型解析为运行时值（例如从 query/body/header/context 提取）。
// 解析顺序由 BindingRegistry 管理，默认“后注册优先”，便于覆盖默认行为。
func (r *Runtime) RegisterBinding(resolver BindingResolver) {
	r.executor.RegisterBinding(resolver)
}

// RegisterBindingFunc 以函数形式注册一个简单的参数绑定规则。
//
// matcher 用于判断某个参数类型是否由该规则处理；resolve 负责生成该参数的运行时值。
// 适合在外部快速扩展 binding，而无需实现完整的 BindingResolver 类型。
func (r *Runtime) RegisterBindingFunc(matcher func(reflect.Type) bool, resolve func(ctx *HandlerContext, paramType reflect.Type) (interface{}, error)) {
	r.executor.RegisterBindingFunc(matcher, resolve)
}

// GetRouter 返回当前 Runtime 使用的 Router。
func (r *Runtime) GetRouter() *Router {
	return r.router
}

// GetPipeline 返回当前 Runtime 使用的 Pipeline。
func (r *Runtime) GetPipeline() *Pipeline {
	return r.pipeline
}

// GetExecutor 返回当前 Runtime 使用的 Executor。
func (r *Runtime) GetExecutor() *Executor {
	return r.executor
}
