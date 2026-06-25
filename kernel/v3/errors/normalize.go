package errors

import (
	"errors"
	"maps"
)

// Normalize 将任意 error 归一化为 KernelError。
//
// 行为：
// - 如果 err 已经是 *KernelError，则补齐缺失字段并合并 meta
// - 否则创建新的 KernelError，并根据 sentinel error 做 Kind 分类
//
// 备注：
// - protocol/routeKey/moduleKey/paramIndex 由 runtime 提供，用于定位“错误发生在哪个协议/路由/模块/参数”
// - meta 用于补充结构化诊断信息（例如 stage、bindingName、paramType、method/path 等）
func Normalize(protocol, routeKey, moduleKey string, err error) *KernelError {
	return NormalizeWithMeta(protocol, routeKey, moduleKey, -1, nil, err)
}

// NormalizeWithMeta 是 Normalize 的增强版：允许指定参数索引并携带 meta。
//
// 如果 err 已经是 *KernelError：
// - 仅补齐缺失字段（不会覆盖已有的 Protocol/RouteKey/ModuleKey/Meta 等）
// - 当 existing.ParamIndex 未设置且传入 paramIndex>=0 时才会写入 ParamIndex
// - meta 会“合并写入” existing.Meta（同 key 会覆盖 existing.Meta 中旧值）
//
// 如果 err 不是 *KernelError：
// - 创建新的 KernelError，并把 err 放入 Cause
// - Kind 会基于 sentinel 错误进行分类（见 switch errors.Is(...)）
func NormalizeWithMeta(protocol, routeKey, moduleKey string, paramIndex int, meta map[string]interface{}, err error) *KernelError {
	if err == nil {
		return nil
	}

	if existing, ok := err.(*KernelError); ok {
		if existing.Protocol == "" {
			existing.Protocol = protocol
		}
		if existing.RouteKey == "" {
			existing.RouteKey = routeKey
		}
		if existing.ModuleKey == "" {
			existing.ModuleKey = moduleKey
		}
		if existing.ParamIndex < 0 && paramIndex >= 0 {
			existing.ParamIndex = paramIndex
		}
		if len(meta) > 0 {
			if existing.Meta == nil {
				existing.Meta = make(map[string]interface{}, len(meta))
			}
			maps.Copy(existing.Meta, meta)
		}
		return existing
	}

	ke := &KernelError{
		Kind:       KindUnknown,
		Cause:      err,
		Message:    err.Error(),
		ParamIndex: paramIndex,
		Meta:       copyMeta(meta),
	}

	if ke.ParamIndex < 0 {
		ke.ParamIndex = -1
	}

	ke.Protocol = protocol
	ke.RouteKey = routeKey
	ke.ModuleKey = moduleKey

	switch {
	case errors.Is(err, ErrRouteNotFound):
		ke.Kind = KindRouteNotFound
	case errors.Is(err, ErrBindingNotFound),
		errors.Is(err, ErrBindingNotSupported),
		errors.Is(err, ErrInvalidParamType):
		ke.Kind = KindBinding
	case errors.Is(err, ErrDIResolveFailed):
		ke.Kind = KindDI
	case errors.Is(err, ErrGuardRejected):
		ke.Kind = KindForbidden
	case errors.Is(err, ErrExecutionFailed):
		ke.Kind = KindExecution
	}

	return ke
}

// copyMeta 复制一份 meta map，避免调用方后续修改影响错误对象内部状态。
//
// 约定：meta 为空或 len==0 时返回 nil（减少分配）。
func copyMeta(meta map[string]interface{}) map[string]interface{} {
	if len(meta) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(meta))
	maps.Copy(out, meta)
	return out
}
