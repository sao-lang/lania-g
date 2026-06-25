package aop

// NestInterceptor 定义拦截器的核心行为：包裹后续链路并决定如何处理结果与错误。
type NestInterceptor interface {
	Intercept(ctx *ExecutionContext, next CallHandler) (interface{}, error)
}

// Interceptor 是框架对“拦截器”对象的统一抽象。
type Interceptor interface {
	NestInterceptor
}

// InterceptorConstructor 用于延迟创建 Interceptor 实例。
type InterceptorConstructor func() Interceptor

// CallHandler 表示“继续执行后续拦截器 / 最终 handler”的句柄。
type CallHandler interface {
	Handle() (interface{}, error)
}

// InterceptorFunc 是 Interceptor 的函数式写法。
type InterceptorFunc func(ctx *ExecutionContext, next CallHandler) (interface{}, error)

// Intercept 让 InterceptorFunc 适配 Interceptor 接口。
//
// next 表示“继续执行后续拦截器/最终 handler”，由 CallHandler.Handle() 触发。
func (f InterceptorFunc) Intercept(ctx *ExecutionContext, next CallHandler) (interface{}, error) {
	return f(ctx, next)
}

// WrapInterceptor 将 Interceptor（对象形式）包装为 InterceptorFunc（函数形式）。
//
// 这样上层 pipeline 可以统一按函数切片执行 interceptors。
func WrapInterceptor(interceptor Interceptor) InterceptorFunc {
	return func(ctx *ExecutionContext, next CallHandler) (interface{}, error) {
		return interceptor.Intercept(ctx, next)
	}
}

// callHandler 是 CallHandler 的默认实现。
type callHandler struct {
	handler func() (interface{}, error)
}

// NewCallHandler 创建一个 CallHandler。
//
// CallHandler 是拦截器链的“继续执行”抽象，拦截器通过 next.Handle() 触发后续链路。
func NewCallHandler(handler func() (interface{}, error)) CallHandler {
	return &callHandler{handler: handler}
}

// Handle 执行底层 handler，并返回其 (result, error)。
func (h *callHandler) Handle() (interface{}, error) {
	return h.handler()
}
