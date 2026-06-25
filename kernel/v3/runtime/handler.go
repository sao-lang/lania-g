// handler.go 定义 runtime 对“可执行目标”的统一封装，以及 handler 元信息模型。
//
// 这一层的重点是把“业务方法”包装成一个稳定运行时对象，
// 让 Router/Executor/Pipeline 不必再关心它最初是 DSL 运行期注册还是编译期产物。
package runtime

import (
	"reflect"
	"sync"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
)

// Handler 是 runtime 对“可执行目标”的统一封装。
//
// 两种模式：
// 1) Instance 模式：直接持有 receiver 实例 + reflect.Method（适合 DSL 运行期注册）
// 2) Token 模式：只持有 ReceiverToken + MethodIndex（receiver 在每次请求时从 ctx.Container 解析）
type Handler struct {
	// ReceiverToken 用于容器管理的 receiver 解析（编译期产物常用）。
	// 它通常是一个 reflect.Type（指针类型），并已注册在模块容器中。
	ReceiverToken interface{}
	Instance      interface{}
	// MethodIndex 是 reflect.Value.Method(i) 使用的方法索引。
	// 对 Token 模式而言，它可以避免每次请求都做 MethodByName 查找。
	// -1 表示未知/未设置，将回退到按方法名查找。
	MethodIndex int
	Method        reflect.Value
	MethodType    reflect.Type
	Meta       *HandlerMeta
}

// HandlerMeta 描述 handler 的编译/运行期元信息，主要用于：
// - Router/协议：RouteKey/Protocol/ModuleKey
// - Binding：ParamTypes/ParamPlans/ParamPipes
// - AOP：middlewares/guards/interceptors/pipes/filters（以及编译期的 CompiledAOP）
// - Response：StatusCode/Headers/Redirect/Render（由各 adapter 解释并落到具体协议响应）
type HandlerMeta struct {
	Name         string
	RouteKey     string
	Protocol     Protocol
	ModuleKey    string
	ParamTypes   []reflect.Type
	ParamPlans   []ParamPlan
	CompiledAOP  *AOPPlan
	ReturnTypes  []reflect.Type
	Middlewares  []aop.MiddlewareFunc
	Guards       []aop.GuardFunc
	Interceptors []aop.InterceptorFunc
	Pipes        []aop.PipeFunc
	ParamPipes   map[int][]aop.PipeFunc
	Filters      []aop.ExceptionFilterFunc
	StatusCode   int
	Headers      map[string]string
	RedirectURL  string
	RedirectCode int
	Render       string
	mu           sync.RWMutex
}

// NewHandler 基于一个 receiver 实例与方法名创建新的处理器（Instance 模式）。
// 更适合运行期直接拿到具体实例的场景，例如手写注册或简单测试。
func NewHandler(instance interface{}, methodName string) (*Handler, error) {
	instanceValue := reflect.ValueOf(instance)
	method := instanceValue.MethodByName(methodName)
	if !method.IsValid() {
		return nil, ErrMethodNotFound
	}

	methodType := method.Type()
	paramTypes := make([]reflect.Type, methodType.NumIn())
	for i := 0; i < methodType.NumIn(); i++ {
		paramTypes[i] = methodType.In(i)
	}

	returnTypes := make([]reflect.Type, methodType.NumOut())
	for i := 0; i < methodType.NumOut(); i++ {
		returnTypes[i] = methodType.Out(i)
	}

	return &Handler{
		ReceiverToken: reflect.TypeOf(instance),
		Instance:   instance,
		MethodIndex:  -1,
		Method:     method,
		MethodType: methodType,
		Meta: &HandlerMeta{
			Name:        methodName,
			ParamTypes:  paramTypes,
			ReturnTypes: returnTypes,
			ParamPipes:  make(map[int][]aop.PipeFunc),
			Headers:     make(map[string]string),
		},
	}, nil
}

// NewHandlerByToken 创建一个“Token 模式”的 Handler。
//
// Token 模式用于“编译期/容器管理”的 receiver：
// - 这里不会保存 receiver 实例（Instance=nil），而是保存 ReceiverToken（通常是 reflect.Type）
// - Invoke 时会从 ctx.Container 里按 token 解析 receiver，然后再调用方法
//
// 注意：
// - token 必须是 reflect.Type（如果不是指针类型会自动转为指针类型）
// - 通过 typ.MethodByName 查找方法，得到的方法签名包含 receiver 作为第一个入参，因此 ParamTypes 需要跳过 receiver
func NewHandlerByToken(token interface{}, methodName string) (*Handler, error) {
	typ, ok := token.(reflect.Type)
	if !ok || typ == nil {
		return nil, ErrInvalidHandler
	}
	if typ.Kind() != reflect.Ptr {
		typ = reflect.PointerTo(typ)
	}
	m, ok := typ.MethodByName(methodName)
	if !ok {
		return nil, ErrMethodNotFound
	}
	methodType := m.Type // 注意：这里的签名包含 receiver 作为第一个入参
	if methodType.NumIn() == 0 {
		return nil, ErrInvalidHandler
	}

	paramTypes := make([]reflect.Type, methodType.NumIn()-1)
	for i := 1; i < methodType.NumIn(); i++ {
		paramTypes[i-1] = methodType.In(i)
	}

	returnTypes := make([]reflect.Type, methodType.NumOut())
	for i := 0; i < methodType.NumOut(); i++ {
		returnTypes[i] = methodType.Out(i)
	}

	return &Handler{
		ReceiverToken: token,
		Instance:      nil,
		MethodIndex:   m.Index,
		// Method 不在编译期持有具体值，而是在每次请求时按容器里的 receiver 重新解析。
		Method:        reflect.Value{},
		MethodType:    methodType,
		Meta: &HandlerMeta{
			Name:        methodName,
			ParamTypes:  paramTypes,
			ReturnTypes: returnTypes,
			ParamPipes:  make(map[int][]aop.PipeFunc),
			Headers:     make(map[string]string),
		},
	}, nil
}

// Invoke 调用 handler 对应的方法，并返回 reflect.Call 的结果切片。
//
// 执行模式：
// - Instance 模式：如果 h.Instance != nil 且 h.Method 有效，直接 h.Method.Call(args)
// - Token 模式：从 ctx.Container.Get(h.ReceiverToken) 解析 receiver，再通过 MethodIndex/Name 找到方法并调用
//
// 错误约定：
// - 若 Token 模式下 ctx 或 ctx.Container 为空，返回 ErrDIResolveFailed
// - 若找不到 receiver 或方法无效，返回对应错误（通常会在 Executor.normalizeError 中被归一化）
func (h *Handler) Invoke(ctx *HandlerContext, args []reflect.Value) ([]reflect.Value, error) {
	if h.Instance != nil && h.Method.IsValid() {
		return h.Method.Call(args), nil
	}
	if ctx == nil || ctx.Container == nil {
		return nil, ErrDIResolveFailed
	}
	receiver, err := ctx.Container.Get(h.ReceiverToken)
	if err != nil {
		return nil, err
	}
	rv := reflect.ValueOf(receiver)
	var method reflect.Value
	if h.MethodIndex >= 0 {
		// MethodIndex 是最快路径，避免每次请求都做字符串方法查找。
		method = rv.Method(h.MethodIndex)
	} else {
		method = rv.MethodByName(h.Meta.Name)
	}
	if !method.IsValid() {
		return nil, ErrMethodNotFound
	}
	return method.Call(args), nil
}

// NumParams 返回 handler 的参数数量（不包含 receiver）。
func (h *Handler) NumParams() int {
	return len(h.Meta.ParamTypes)
}

// NumReturns 返回 handler 的返回值数量。
func (h *Handler) NumReturns() int {
	return len(h.Meta.ReturnTypes)
}

// WithMiddleware 为当前 handler 增加中间件（middlewares）。
//
// middlewares 会在 Pipeline.Run 中与全局 middlewares 合并后执行（除非该 handler 有编译期 AOP 计划）。
func (hm *HandlerMeta) WithMiddleware(middlewares ...aop.MiddlewareFunc) *HandlerMeta {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.Middlewares = append(hm.Middlewares, middlewares...)
	return hm
}

// WithGuards 为当前 handler 增加守卫（guards）。
//
// guards 在进入 handler 调用前执行；任意 guard 返回 (false, nil) 会中断执行并返回 ErrGuardRejected。
func (hm *HandlerMeta) WithGuards(guards ...aop.GuardFunc) *HandlerMeta {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.Guards = append(hm.Guards, guards...)
	return hm
}

// WithInterceptors 为当前 handler 增加拦截器（interceptors）。
//
// interceptor 会包裹 handler 调用，适合做 tracing/metrics/caching/retry 等横切逻辑。
func (hm *HandlerMeta) WithInterceptors(interceptors ...aop.InterceptorFunc) *HandlerMeta {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.Interceptors = append(hm.Interceptors, interceptors...)
	return hm
}

// WithPipes 为当前 handler 增加 Pipe（管道）。
//
// Pipe 会在 Pipeline.Run 中对入参/返回值做 Transform（见 pipeline.go）。
func (hm *HandlerMeta) WithPipes(pipes ...aop.PipeFunc) *HandlerMeta {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.Pipes = append(hm.Pipes, pipes...)
	return hm
}

// WithParamPipes 为某个参数索引追加参数级 Pipe。
//
// 参数级 Pipe 的执行发生在 Executor.resolveArgument 中：
// - 先完成 binding/DI 得到值
// - 再对该值依次执行 ParamPipes[paramIndex]
func (hm *HandlerMeta) WithParamPipes(paramIndex int, pipes ...aop.PipeFunc) *HandlerMeta {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.ParamPipes[paramIndex] = append(hm.ParamPipes[paramIndex], pipes...)
	return hm
}

// WithFilters 为当前 handler 增加异常过滤器（filters）。
//
// filters 只在出现 error 时执行；返回 nil 表示错误已被消费（例如已写入响应）。
func (hm *HandlerMeta) WithFilters(filters ...aop.ExceptionFilterFunc) *HandlerMeta {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.Filters = append(hm.Filters, filters...)
	return hm
}
