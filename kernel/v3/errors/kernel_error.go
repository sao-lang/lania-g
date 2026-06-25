package errors

import "fmt"

// Kind 表示框架内部对错误的高层分类。
//
// adapter 往往不会直接依赖具体的底层错误文本，而是先基于 Kind
// 决定应该映射成哪类协议错误，例如 HTTP 状态码或 gRPC code。
type Kind string

const (
	// KindUnknown 表示未命中任何已知分类规则的错误。
	KindUnknown       Kind = "Unknown"
	// KindRouteNotFound 表示路由匹配失败。
	KindRouteNotFound Kind = "RouteNotFound"
	// KindBinding 表示参数绑定或转换阶段出错。
	KindBinding       Kind = "Binding"
	// KindValidation 表示数据校验失败。
	KindValidation    Kind = "Validation"
	// KindDI 表示依赖注入解析失败。
	KindDI            Kind = "DI"
	// KindExecution 表示 handler 或执行链本身发生失败。
	KindExecution     Kind = "Execution"
	// KindUnauthorized 表示认证失败或未登录。
	KindUnauthorized  Kind = "Unauthorized"
	// KindForbidden 表示已识别调用方，但当前操作被拒绝。
	KindForbidden     Kind = "Forbidden"
)

// KernelError 是框架内部的结构化错误，用于跨协议映射与诊断。
//
// 设计目标：
// - 将“错误发生在哪个协议/路由/模块/参数阶段”编码在错误对象中
// - 让 adapter 可以把 KernelError 映射到协议侧的 status/code/message
// - Meta 用于承载可观测性信息（stage、bindingName、paramType、method/path 等）
type KernelError struct {
	Kind       Kind
	Protocol   string
	RouteKey   string
	ModuleKey  string
	ParamIndex int
	Message    string
	Cause      error
	Meta       map[string]interface{}
}

// Error 实现 error 接口，返回“对外可读”的错误消息。
//
// 优先级：
// 1) Message：明确指定的错误消息（通常适合直接暴露给调用方或用于日志）
// 2) Cause.Error()：若未设置 Message，则回退到底层原因错误
// 3) Kind：兜底输出一种通用消息，保证不会返回空字符串
//
// 注意：KernelError 的结构化信息（Protocol/RouteKey/Meta 等）不在 Error() 中展开，
// 上层如需观测/诊断应直接读取字段。
func (e *KernelError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return fmt.Sprintf("kernel error: %s", e.Kind)
}

// Unwrap 返回底层原因错误，使 errors.Is / errors.As 能穿透 KernelError。
//
// 约定：Normalize/NormalizeWithMeta 会把原始错误放入 Cause，便于调用方做 sentinel 匹配与类型断言。
func (e *KernelError) Unwrap() error { return e.Cause }
