// resolver_auth.go 实现 HTTP 鉴权相关 binding 的解析逻辑。
package http

import "github.com/sao-lang/lania-g/kernel/v3/runtime"

// 这组鉴权 resolver 都遵循同一个约定：
// adapter/guard 若提前把鉴权结果写进 metadata，就原样投影为 wrapper；
// 否则返回零值，让 handler 可以按“匿名请求”分支继续处理。

func resolveAuthUser(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	user, _ := ctx.Get(MetadataKeyAuthUser)
	return AuthUser{Value: user}, nil
}

func resolveAuthUserID(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	idAny, _ := ctx.Get(MetadataKeyAuthUserID)
	if v, ok := idAny.(string); ok {
		return AuthUserID{Value: v}, nil
	}
	return AuthUserID{Value: ""}, nil
}

func resolveAuthToken(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	tokAny, _ := ctx.Get(MetadataKeyAuthToken)
	if t, ok := tokAny.(AuthToken); ok {
		return t, nil
	}
	if m, ok := tokAny.(map[string]any); ok {
		// 保留 map 兼容分支，是为了兼容旧中间件只写入裸 map 的历史行为。
		value, _ := m["value"].(string)
		claims, _ := m["claims"].(map[string]any)
		return AuthToken{Value: value, Claims: claims}, nil
	}
	return AuthToken{Value: "", Claims: nil}, nil
}

func resolveAuthOptionalUser(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	val, _ := ctx.Get(MetadataKeyAuthOptionalUser)
	if u, ok := val.(AuthOptionalUser); ok {
		return u, nil
	}
	if m, ok := val.(map[string]any); ok {
		auth, _ := m["authenticated"].(bool)
		return AuthOptionalUser{Value: m["value"], Authenticated: auth}, nil
	}
	return AuthOptionalUser{Value: nil, Authenticated: false}, nil
}

func resolveAuthOptionalToken(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	val, _ := ctx.Get(MetadataKeyAuthOptionalToken)
	if t, ok := val.(AuthOptionalToken); ok {
		return t, nil
	}
	if m, ok := val.(map[string]any); ok {
		value, _ := m["value"].(string)
		claims, _ := m["claims"].(map[string]any)
		auth, _ := m["authenticated"].(bool)
		return AuthOptionalToken{Value: value, Claims: claims, Authenticated: auth}, nil
	}
	return AuthOptionalToken{Value: "", Claims: nil, Authenticated: false}, nil
}

func resolveAuthRole(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	// 目前 AuthRole/AuthPermission 只占位暴露类型，
	// 真正的角色/权限判断仍应由 guard/interceptor 承担。
	return AuthRole{}, nil
}

func resolveAuthPermission(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	return AuthPermission{}, nil
}
