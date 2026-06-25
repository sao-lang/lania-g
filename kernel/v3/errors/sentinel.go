package errors

import stderrors "errors"

// 这些哨兵错误用于在 runtime / compiler / adapter 各层之间传递统一语义。
//
// 上层通常通过 `errors.Is` 判断这些错误，再决定：
// - 是否要转成 `KernelError`
// - 是否要映射为协议侧状态码/错误码
// - 是否要追加更具体的诊断信息
var (
	// ErrMethodNotFound 表示尝试通过反射/索引调用方法时未找到目标方法。
	ErrMethodNotFound      = stderrors.New("method not found")
	// ErrRouteNotFound 表示 Router 未能匹配到任何 handler。
	ErrRouteNotFound       = stderrors.New("route not found")
	// ErrInvalidProtocol 表示路由 key 或协议字段不符合约定格式。
	ErrInvalidProtocol     = stderrors.New("invalid protocol")
	// ErrBindingNotFound 表示没有任何 BindingResolver 能处理该参数类型。
	ErrBindingNotFound     = stderrors.New("binding resolver not found for type")
	// ErrBindingNotSupported 表示某个 binding resolver 命中类型但不支持当前协议。
	ErrBindingNotSupported = stderrors.New("binding not supported for protocol")
	// ErrInvalidHandler 表示 handler 定义不合法（例如 token 类型不对、方法签名异常等）。
	ErrInvalidHandler      = stderrors.New("invalid handler")
	// ErrExecutionFailed 表示执行链路发生非预期失败（例如 panic 被 recover，或 handler 调用失败）。
	ErrExecutionFailed     = stderrors.New("execution failed")
	// ErrGuardRejected 表示守卫拒绝继续执行（常用于鉴权失败/无权限）。
	ErrGuardRejected       = stderrors.New("guard rejected access")
	// ErrMiddlewareAborted 表示中间件主动中断执行链路。
	ErrMiddlewareAborted   = stderrors.New("middleware aborted")
	// ErrInvalidParamType 表示参数绑定/pipe 输出的值无法赋值或转换为目标参数类型。
	ErrInvalidParamType    = stderrors.New("invalid parameter type")
	// ErrInvalidReturnType 表示返回值类型不符合框架约定（由 adapter/runtime 在需要时使用）。
	ErrInvalidReturnType   = stderrors.New("invalid return type")
	// ErrDIResolveFailed 表示从 DI 容器解析依赖失败（找不到 provider 或构造失败）。
	ErrDIResolveFailed     = stderrors.New("di resolve failed")
)
