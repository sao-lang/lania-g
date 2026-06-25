// middleware.go 实现 auth 集成的运行时中间件与上下文透传逻辑。
package auth

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// Middleware 创建一个会在请求开始阶段执行认证的中间件。
func Middleware(service *Service) aop.MiddlewareFunc {
	return func(execCtx *aop.ExecutionContext, next func() error) error {
		hc, ok := execCtx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return next()
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
func RequireAuthenticated(service *Service) aop.GuardFunc {
	return func(execCtx *aop.ExecutionContext) (bool, error) {
		hc, ok := execCtx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return false, nil
		}
		principal, err := ensurePrincipal(service, hc)
		if err != nil {
			return false, err
		}
		return principal != nil && principal.Subject != "", nil
	}
}

// RequireRoles 创建一个要求主体至少具备任一角色的 guard。
func RequireRoles(service *Service, roles ...string) aop.GuardFunc {
	required := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		required[strings.TrimSpace(role)] = struct{}{}
	}
	return func(execCtx *aop.ExecutionContext) (bool, error) {
		hc, ok := execCtx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return false, nil
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
func RequireTenant(service *Service) aop.GuardFunc {
	return func(execCtx *aop.ExecutionContext) (bool, error) {
		hc, ok := execCtx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return false, nil
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

// Install 把认证中间件安装到支持全局 middleware 的对象上。
func Install(into interface {
	UseGlobalMiddlewares(...aop.MiddlewareFunc)
}, service *Service) {
	if into == nil || service == nil {
		return
	}
	into.UseGlobalMiddlewares(Middleware(service))
}

// InstallOnFactory 把认证中间件安装到暴露 `UseGlobalMiddleware` 的 factory 上。
func InstallOnFactory(factory any, service *Service) error {
	if factory == nil || service == nil {
		return nil
	}
	value := reflect.ValueOf(factory)
	method := value.MethodByName("UseGlobalMiddleware")
	if !method.IsValid() {
		return fmt.Errorf("factory does not expose auth install hook")
	}
	method.Call([]reflect.Value{reflect.ValueOf(Middleware(service))})
	return nil
}

// RequireClaims 创建一个要求 claims 包含指定键值对的 guard。
func RequireClaims(service *Service, claims map[string]interface{}) aop.GuardFunc {
	return func(execCtx *aop.ExecutionContext) (bool, error) {
		hc, ok := execCtx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return false, nil
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
func RequireAnyRole(service *Service, roles ...string) aop.GuardFunc {
	return RequireRoles(service, roles...)
}

// RequireAllRoles 创建一个要求主体具备全部角色的 guard。
func RequireAllRoles(service *Service, roles ...string) aop.GuardFunc {
	required := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		required[strings.TrimSpace(role)] = struct{}{}
	}
	return func(execCtx *aop.ExecutionContext) (bool, error) {
		hc, ok := execCtx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return false, nil
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
