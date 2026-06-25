// binding.go 实现 auth integration 暴露给 handler 的 binding 包装与 resolver。
package auth

import (
	"reflect"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// RegisterBindings 注册认证相关的注入包装 binding。
func RegisterBindings(reg *registry.Registry) {
	if reg == nil {
		RegisterBindingsCompat()
		return
	}
	registerBindings(reg)
}

// RegisterBindingsCompat 显式保留“写入全局 registry”的兼容 binding 注册入口。
func RegisterBindingsCompat() {
	registerBindings(registry.GlobalWithUsage("integrations/auth.RegisterBindingsCompat"))
}

func registerBindings(reg *registry.Registry) {
	reg.RegisterBindings(
		&resolver{name: "AuthInjectPrincipal", match: matchWrapper("InjectPrincipal"), resolve: resolveInjectPrincipal},
		&resolver{name: "AuthInjectTenant", match: matchWrapper("InjectTenant"), resolve: resolveInjectTenant},
		&resolver{name: "AuthInjectClaims", match: matchWrapper("InjectClaims"), resolve: resolveInjectClaims},
		&resolver{name: "AuthPrincipalRef", match: matchWrapper("PrincipalRef"), resolve: resolveInjectPrincipal},
	)
}

type resolver struct {
	name    string
	match   func(reflect.Type) (runtime.WrapperDescriptor, bool)
	resolve func(*runtime.HandlerContext, runtime.WrapperDescriptor) (any, error)
}

// Name 返回当前认证 binding resolver 的名称。
func (r *resolver) Name() string { return r.name }

// AllowedProtocols 返回该 resolver 允许的协议集合；`nil` 表示不限制。
func (r *resolver) AllowedProtocols() map[runtime.Protocol]bool { return nil }

// Match 判断某个类型是否应由当前 resolver 处理。
func (r *resolver) Match(t reflect.Type) (runtime.WrapperDescriptor, bool) { return r.match(t) }

// Resolve 根据 wrapper 描述解析出实际注入值。
func (r *resolver) Resolve(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	return r.resolve(ctx, desc)
}

func matchWrapper(base string) func(reflect.Type) (runtime.WrapperDescriptor, bool) {
	return func(t reflect.Type) (runtime.WrapperDescriptor, bool) {
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct || t.PkgPath() != packagePath() || t.NumField() == 0 {
			return runtime.WrapperDescriptor{}, false
		}
		name := t.Name()
		if idx := strings.Index(name, "["); idx >= 0 {
			name = name[:idx]
		}
		if name != base {
			return runtime.WrapperDescriptor{}, false
		}
		return runtime.WrapperDescriptor{Kind: base, WrapperType: t, InnerType: t.Field(0).Type}, true
	}
}

func resolveInjectPrincipal(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	principal := Current(ctx)
	return wrap(desc.WrapperType, reflect.ValueOf(principal))
}

func resolveInjectTenant(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	return wrap(desc.WrapperType, reflect.ValueOf(CurrentTenant(ctx)))
}

func resolveInjectClaims(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	claims := map[string]any{}
	if principal := Current(ctx); principal != nil && principal.Claims != nil {
		for k, v := range principal.Claims {
			claims[k] = v
		}
	}
	return wrap(desc.WrapperType, reflect.ValueOf(claims))
}

func wrap(wrapperType reflect.Type, value reflect.Value) (any, error) {
	if wrapperType.Kind() == reflect.Ptr {
		wrapperType = wrapperType.Elem()
	}
	target := reflect.New(wrapperType).Elem()
	target.Field(0).Set(value)
	return target.Interface(), nil
}

func packagePath() string {
	return reflect.TypeOf(InjectPrincipal{}).PkgPath()
}
