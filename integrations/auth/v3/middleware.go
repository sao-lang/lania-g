// middleware.go 实现 auth 集成的运行时中间件与上下文透传逻辑。
package auth

import (
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// Middleware 创建一个在请求开始阶段执行认证的中间件。
//
// 它不需要外部传入 Service：Service 从请求容器解析。
func Middleware() aop.MiddlewareFunc {
	return func(execCtx *aop.ExecutionContext, next func() error) error {
		hc, ok := execCtx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return next()
		}
		service, err := serviceOf(execCtx)
		if err != nil {
			return err
		}
		principal, err := service.Authenticate(hc)
		if err != nil {
			return err
		}
		if principal != nil {
			applyPrincipal(hc, principal)
		}
		return next()
	}
}

// RequireAuthenticated 创建一个要求当前请求已认证的 guard。
func RequireAuthenticated() aop.GuardFunc {
	return func(execCtx *aop.ExecutionContext) (bool, error) {
		hc, ok := execCtx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return false, nil
		}
		service, err := serviceOf(execCtx)
		if err != nil {
			return false, err
		}
		principal, err := ensurePrincipal(service, hc)
		if err != nil {
			return false, err
		}
		return principal != nil && principal.Subject != "", nil
	}
}

// RequireRoles 创建一个要求主体至少具备任一角色的 guard。
func RequireRoles(roles ...string) aop.GuardFunc {
	required := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		required[strings.TrimSpace(role)] = struct{}{}
	}
	return func(execCtx *aop.ExecutionContext) (bool, error) {
		hc, ok := execCtx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return false, nil
		}
		service, err := serviceOf(execCtx)
		if err != nil {
			return false, err
		}
		principal, err := ensurePrincipal(service, hc)
		if err != nil || principal == nil {
			return false, err
		}
		for _, role := range principal.Roles {
			if _, ok := required[role]; ok {
				return true, nil
			}
		}
		return false, nil
	}
}

// RequireTenant 创建一个要求当前请求具备租户信息的 guard。
func RequireTenant() aop.GuardFunc {
	return func(execCtx *aop.ExecutionContext) (bool, error) {
		hc, ok := execCtx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return false, nil
		}
		service, err := serviceOf(execCtx)
		if err != nil {
			return false, err
		}
		principal, err := ensurePrincipal(service, hc)
		if err != nil {
			return false, err
		}
		if principal != nil && principal.TenantID != "" {
			return true, nil
		}
		return CurrentTenant(hc) != "", nil
	}
}

// RequireClaims 创建一个要求 claims 包含指定键值对的 guard。
func RequireClaims(claims map[string]interface{}) aop.GuardFunc {
	return func(execCtx *aop.ExecutionContext) (bool, error) {
		hc, ok := execCtx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return false, nil
		}
		service, err := serviceOf(execCtx)
		if err != nil {
			return false, err
		}
		principal, err := ensurePrincipal(service, hc)
		if err != nil || principal == nil {
			return false, err
		}
		for key, expected := range claims {
			if actual, ok := principal.Claims[key]; !ok || actual != expected {
				return false, nil
			}
		}
		return true, nil
	}
}

// RequireAnyRole 是 `RequireRoles` 的语义别名。
func RequireAnyRole(roles ...string) aop.GuardFunc {
	return RequireRoles(roles...)
}

// RequireAllRoles 创建一个要求主体具备全部角色的 guard。
func RequireAllRoles(roles ...string) aop.GuardFunc {
	required := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		required[strings.TrimSpace(role)] = struct{}{}
	}
	return func(execCtx *aop.ExecutionContext) (bool, error) {
		hc, ok := execCtx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return false, nil
		}
		service, err := serviceOf(execCtx)
		if err != nil {
			return false, err
		}
		principal, err := ensurePrincipal(service, hc)
		if err != nil || principal == nil {
			return false, err
		}
		roleMap := make(map[string]struct{}, len(principal.Roles))
		for _, role := range principal.Roles {
			roleMap[role] = struct{}{}
		}
		for role := range required {
			if _, ok := roleMap[role]; !ok {
				return false, nil
			}
		}
		return true, nil
	}
}

// ChainGuards 把多个 guard 串成一个按顺序执行的组合 guard。
func ChainGuards(guards ...aop.GuardFunc) aop.GuardFunc {
	return func(execCtx *aop.ExecutionContext) (bool, error) {
		for _, guard := range guards {
			allowed, err := guard(execCtx)
			if err != nil || !allowed {
				return allowed, err
			}
		}
		return true, nil
	}
}

// serviceOf 从请求容器解析 auth Service。
func serviceOf(ctx *aop.ExecutionContext) (*Service, error) {
	if ctx == nil {
		return nil, aop.InternalServerErrorException("auth: execution context unavailable")
	}
	container, ok := ctx.Container.(*di.Container)
	if !ok || container == nil {
		return nil, aop.InternalServerErrorException("auth: request container unavailable")
	}
	service, err := di.GetByType[*Service](container)
	if err != nil || service == nil {
		return nil, aop.InternalServerErrorException("auth: service not registered")
	}
	return service, nil
}
