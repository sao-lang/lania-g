// binding.go 实现“从请求级 DI 容器直接注入对象”的 binding 注册辅助。
//
// 它是 integration 层把容器中的服务/上下文对象暴露为 handler 参数的一条通用桥。
package integration

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// RegisterContainerBindings 注册一组精确类型 binding resolver，
// 用于从当前请求容器中获取实例。
//
// 中文说明：
// RegisterContainerBindings 注册一组“从请求级 DI 容器读取实例”的精确类型绑定：
// - 当 handler 参数类型 == entry.Token 时命中
// - Resolve 时从 ctx.Container.Get(token) 返回实例
//
// 常见用途：将一些“上下文对象/服务对象”直接作为 handler 入参注入。
func RegisterContainerBindings(reg *registry.Registry, entries ...BindingEntry) {
	if reg == nil {
		RegisterContainerBindingsCompat(entries...)
		return
	}
	registerContainerBindings(reg, entries...)
}

// RegisterContainerBindingsCompat 显式保留“写入全局 registry”的兼容容器 binding 注册入口。
func RegisterContainerBindingsCompat(entries ...BindingEntry) {
	registerContainerBindings(registry.GlobalWithUsage("core/integration.RegisterContainerBindingsCompat"), entries...)
}

func registerContainerBindings(reg *registry.Registry, entries ...BindingEntry) {
	resolvers := make([]runtime.BindingResolver, 0, len(entries))
	for _, entry := range entries {
		if entry.Token == nil {
			continue
		}
		resolvers = append(resolvers, &containerBindingResolver{
			name:   entry.Name,
			target: entry.Token,
		})
	}
	if len(resolvers) > 0 {
		reg.RegisterBindings(resolvers...)
	}
}

// NewBindingEntry 创建一个 BindingEntry（显式传入 reflect.Type token）。
func NewBindingEntry(name string, token reflect.Type) BindingEntry {
	return BindingEntry{Name: name, Token: token}
}

// NewBindingEntryFor 创建一个 BindingEntry，并用泛型参数 T 推导 token（reflect.Type）。
//
// token 形态为 T 的类型（非指针），用于与 handler 参数类型做精确匹配。
func NewBindingEntryFor[T any](name string) BindingEntry {
	return BindingEntry{Name: name, Token: reflect.TypeFor[T]()}
}

// BindingEntry 描述一个需要注册到 binding 系统中的“类型 -> 名称”映射。
type BindingEntry struct {
	Name  string
	Token reflect.Type
}

// containerBindingResolver 是一个“从请求容器取值”的精确类型 binding resolver。
// 它不做任何协议判断，也不做模糊匹配，命中条件只有“参数类型完全一致”。
type containerBindingResolver struct {
	name   string
	target reflect.Type
}

// Name 返回该 resolver 的名字（用于诊断与错误信息）。
func (r *containerBindingResolver) Name() string { return r.name }

// AllowedProtocols 返回 nil 表示不限制协议（对所有协议都允许）。
func (r *containerBindingResolver) AllowedProtocols() map[runtime.Protocol]bool { return nil }

// Match 仅在参数类型与 target 完全相等时命中（精确类型匹配）。
func (r *containerBindingResolver) Match(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	if t != r.target {
		return runtime.WrapperDescriptor{}, false
	}
	return runtime.WrapperDescriptor{Kind: r.name, WrapperType: t, InnerType: t}, true
}

// Resolve 从当前请求的 ctx.Container 中获取实例。
//
// 注意：该 binding 依赖 request container，因此若 ctx 或 ctx.Container 为空会返回错误。
func (r *containerBindingResolver) Resolve(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (interface{}, error) {
	if ctx == nil || ctx.Container == nil {
		return nil, fmt.Errorf("%s binding requires request container", r.name)
	}
	return ctx.Container.Get(desc.WrapperType)
}
