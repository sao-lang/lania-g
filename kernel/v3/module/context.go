// context.go 定义模块上下文。
//
// 模块上下文是集成/协议模块在 Init 阶段的唯一“外部能力入口”：
//   - 访问本模块容器
//   - 向应用声明 binding 与协议声明
//   - 贡献全局 AOP
//
// 它取代了旧版 SetRegistry 暗通道：模块不再需要判断 registry 是否为 nil，
// 也不需要关心全局单例。
package module

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// Context 是模块在 Init 阶段可见的上下文。
type Context struct {
	module   Module
	key      string
	registry *registry.Registry
	aop      AOP
}

// newContext 由 ModuleLoader 创建，外部不应直接构造。
func newContext(mod Module, reg *registry.Registry) *Context {
	key := ""
	if mod != nil {
		key = reflect.TypeOf(mod).String()
	}
	return &Context{module: mod, key: key, registry: reg}
}

// Module 返回当前模块。
func (c *Context) Module() Module { return c.module }

// Key 返回模块标识（与编译器使用的 moduleKey 一致）。
func (c *Context) Key() string { return c.key }

// Container 返回模块容器。
func (c *Context) Container() *di.Container {
	if c == nil || c.module == nil {
		return nil
	}
	return c.module.Container()
}

// Registry 返回应用级声明仓库。
//
// 它只在协议插件与集成模块内部使用；业务代码不应依赖该方法。
func (c *Context) Registry() *registry.Registry { return c.registry }

// Bind 声明“worker 参数类型 -> 请求容器实例”的注入绑定。
//
// 等价于旧版的 RegisterContainerBindings：handler 参数类型与 token 完全一致时，
// 运行期从请求容器取值注入。
func (c *Context) Bind(name string, tokens ...any) {
	if c == nil || c.registry == nil {
		return
	}
	for _, token := range tokens {
		typ := tokenType(token)
		if typ == nil {
			continue
		}
		c.registry.RegisterBindings(&containerBinding{name: name, target: typ})
	}
}

// containerBinding 是“从请求容器取值”的精确类型 binding resolver。
type containerBinding struct {
	name   string
	target reflect.Type
}

func (b *containerBinding) Name() string { return b.name }

// AllowedProtocols 返回 nil 表示不限制协议。
func (b *containerBinding) AllowedProtocols() map[runtime.Protocol]bool { return nil }

// Match 仅在参数类型与目标类型完全相等时命中。
func (b *containerBinding) Match(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	if t != b.target {
		return runtime.WrapperDescriptor{}, false
	}
	return runtime.WrapperDescriptor{Kind: b.name, WrapperType: t, InnerType: t}, true
}

// Resolve 从请求容器中取实例。
func (b *containerBinding) Resolve(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if ctx == nil || ctx.Container == nil {
		return nil, fmt.Errorf("%s binding requires request container", b.name)
	}
	return ctx.Container.Get(desc.WrapperType)
}

// Use 追加上报给应用的全局 AOP。
//
// 等价于 NestJS 的 APP_GUARD / APP_INTERCEPTOR / APP_PIPE / APP_FILTER 提供者。
func (c *Context) Use(contribution AOP) {
	if c == nil {
		return
	}
	c.aop.Middlewares = append(c.aop.Middlewares, contribution.Middlewares...)
	c.aop.Guards = append(c.aop.Guards, contribution.Guards...)
	c.aop.Interceptors = append(c.aop.Interceptors, contribution.Interceptors...)
	c.aop.Pipes = append(c.aop.Pipes, contribution.Pipes...)
	c.aop.Filters = append(c.aop.Filters, contribution.Filters...)
}

// AOP 返回本模块在 Init 阶段追加的全局 AOP。
func (c *Context) AOP() AOP {
	if c == nil {
		return AOP{}
	}
	return c.aop
}

// RegisterDecl 把协议声明写入本模块所属的声明仓库。
func (c *Context) RegisterDecl(protocol, kind string, items ...any) {
	if c == nil || c.registry == nil || len(items) == 0 {
		return
	}
	c.registry.RegisterDecl(protocol, kind, items...)
}

// tokenType 把 token 归一化为 reflect.Type。
func tokenType(token any) reflect.Type {
	switch value := token.(type) {
	case reflect.Type:
		return value
	case string:
		// 字符串 token 不是类型，无法参与类型匹配；这里显式报错式返回 nil，
		// 避免静默注册一个永不命中的 binding。
		return nil
	default:
		t := reflect.TypeOf(token)
		if t == nil {
			return nil
		}
		if t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Struct {
			return t.Elem()
		}
		return t
	}
}

// String 输出用于诊断。
func (c *Context) String() string {
	if c == nil {
		return "<nil module context>"
	}
	return fmt.Sprintf("module context(%s)", c.key)
}
