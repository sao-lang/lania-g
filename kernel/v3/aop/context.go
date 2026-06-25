package aop

import (
	"context"
)

// ExecutionContext 是 AOP 层统一使用的执行上下文。
//
// 它把 runtime、adapter 与协议侧的信息收拢到一个无协议依赖的结构中，
// 让 middleware / guard / interceptor / pipe / filter 可以用同一套方式读取上下文。
type ExecutionContext struct {
	Context        context.Context
	HandlerContext interface{}
	Handler        interface{}
	Class          interface{}
	Request        interface{}
	Response       interface{}
	Container      interface{}
	Protocol       string
	RouteKey       string
	ModuleKey      string
}

// NewExecutionContext 创建一个 AOP 执行上下文（ExecutionContext）。
//
// 该结构用于把 runtime 层的 HandlerContext/Handler 等信息“投影”到 AOP 层统一使用的上下文中，
// middleware/guard/interceptor/pipe/filter 都只依赖 ExecutionContext，避免与具体协议/运行时实现耦合。
//
// 参数说明：
// - ctx：底层 context.Context（deadline/cancel/value 等）
// - handlerContext：运行时的请求上下文（通常是 *runtime.HandlerContext），但 AOP 层用 interface{} 避免循环依赖
// - handler：当前匹配到的 handler（通常是 *runtime.Handler），同样用 interface{} 解耦
// - class：handler 的 receiver 实例或 token（由 runtime 决定，便于拦截器拿到“类”的概念）
func NewExecutionContext(ctx context.Context, handlerContext interface{}, handler interface{}, class interface{}) *ExecutionContext {
	return &ExecutionContext{
		Context:        ctx,
		HandlerContext: handlerContext,
		Handler:        handler,
		Class:          class,
	}
}

// GetRequest 返回当前请求对象。
//
// 具体类型由 runtime/adapter 填充：
// - 通常是 runtime 的统一 Request 抽象，或协议原生对象的包装
// - AOP 层不假定其具体类型，拦截器需要自行断言
func (ctx *ExecutionContext) GetRequest() interface{} {
	return ctx.Request
}

// GetResponse 返回当前响应对象。
//
// 具体类型由 runtime/adapter 填充；AOP 层只负责透传，便于 filter/interceptor 写入响应相关信息。
func (ctx *ExecutionContext) GetResponse() interface{} {
	return ctx.Response
}
