// errors.go 统一复出 runtime 层最常用的哨兵错误。
//
// 这里不重新定义错误值，而是直接别名 `core/errors`，
// 目的是让 runtime、adapter、application、业务侧在比较错误时始终使用同一组语义。
package runtime

import kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"

// 这些错误大体覆盖三类场景：
// - 路由/handler 解析失败
// - binding/参数解析失败
// - AOP/执行/DI 失败
var (
	ErrMethodNotFound      = kerrors.ErrMethodNotFound
	ErrRouteNotFound       = kerrors.ErrRouteNotFound
	ErrInvalidProtocol     = kerrors.ErrInvalidProtocol
	ErrBindingNotFound     = kerrors.ErrBindingNotFound
	ErrBindingNotSupported = kerrors.ErrBindingNotSupported
	ErrInvalidHandler      = kerrors.ErrInvalidHandler
	ErrExecutionFailed     = kerrors.ErrExecutionFailed
	ErrGuardRejected       = kerrors.ErrGuardRejected
	ErrMiddlewareAborted   = kerrors.ErrMiddlewareAborted
	ErrInvalidParamType    = kerrors.ErrInvalidParamType
	ErrInvalidReturnType   = kerrors.ErrInvalidReturnType
	ErrDIResolveFailed     = kerrors.ErrDIResolveFailed
)
