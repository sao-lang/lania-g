// middleware.go 定义中间件抽象，以及「按路由范围声明中间件」的消费者模型。
package aop

import (
	"errors"
	"strings"
)

// NestMiddleware 定义中间件的核心行为：在 handler 执行前后包裹一层通用逻辑。
type NestMiddleware interface {
	Use(ctx *ExecutionContext, next func() error) error
}

// Middleware 是框架对“中间件”对象的统一抽象。
type Middleware interface {
	NestMiddleware
}

// MiddlewareConstructor 用于延迟创建 Middleware 实例。
//
// 构造器在编译期执行一次，得到的结果会被该路由的所有请求复用。
type MiddlewareConstructor func() Middleware

// MiddlewareFunc 是 Middleware 的函数式写法。
//
// 它同时实现了 Middleware，因此可以直接用于 UseMiddlewares / MiddlewareConsumer.Apply。
type MiddlewareFunc func(ctx *ExecutionContext, next func() error) error

// Use 让 MiddlewareFunc 适配 Middleware 接口。
//
// 约定：
// - next() 表示继续执行后续 middleware/最终 handler
// - middleware 可以选择不调用 next() 以中断链路（通常返回一个 error 表示中断原因）
func (f MiddlewareFunc) Use(ctx *ExecutionContext, next func() error) error {
	return f(ctx, next)
}

// MiddlewareRouteConfigurator 描述一条“中间件 → 路由范围”绑定。
//
// ForRoutes / Exclude 可反复调用，效果为累加：
// - Include 为空表示不做白名单限制（默认对所有路由生效）
// - Exclude 的优先级高于 Include
type MiddlewareRouteConfigurator interface {
	// ForRoutes 追加生效路由（白名单）。
	ForRoutes(routes ...string) MiddlewareRouteConfigurator
	// Exclude 追加不生效路由（黑名单）。
	Exclude(routes ...string) MiddlewareRouteConfigurator
}

// MiddlewareConsumer 由模块在 Configure 中接收，用于声明式注册中间件。
//
// 每次 Apply 产生一条独立绑定，因此可以给不同的中间件配置不同的路由范围，
// 这等价于 NestJS 的 `consumer.apply(A).forRoutes(...)`。
type MiddlewareConsumer interface {
	Apply(middlewares ...any) MiddlewareRouteConfigurator
}

// MiddlewareConfigurable 表示某个模块可以声明式注册中间件。
type MiddlewareConfigurable interface {
	Configure(consumer MiddlewareConsumer)
}

// MiddlewareModule 是 MiddlewareConfigurable 的别名。
type MiddlewareModule = MiddlewareConfigurable

// RouteInfo 描述一条路由的基础信息。
type RouteInfo struct {
	Path   string
	Method string
}

// ScopedMiddleware 是一组中间件与其生效范围的快照。
type ScopedMiddleware struct {
	Middlewares   []MiddlewareFunc
	IncludeRoutes []string
	ExcludeRoutes []string
}

// ShouldApply 判断给定方法 + 路径是否应应用这组中间件。
//
// method 为空表示该协议没有方法概念（此时只有不带方法前缀的规则可能命中）。
func (s ScopedMiddleware) ShouldApply(method, path string) bool {
	for _, rule := range s.ExcludeRoutes {
		if matchRouteRule(rule, method, path) {
			return false
		}
	}
	if len(s.IncludeRoutes) == 0 {
		return true
	}
	for _, rule := range s.IncludeRoutes {
		if matchRouteRule(rule, method, path) {
			return true
		}
	}
	return false
}

// matchRouteRule 匹配一条路由规则。
//
// 规则可以只写路径（`/users/*`），也可以带方法前缀（`GET /users`、`ALL /admin/*`）。
// 路径支持：
// - `*` / `**`：匹配全部
// - 末尾 `*`：前缀匹配（`/users/*` 同时命中 `/users` 与其子路径）
// - `:name` 或 `{name}`：匹配单个路径段（`/users/:id`）
// - 其余按路径段精确匹配
func matchRouteRule(rule, method, path string) bool {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return false
	}
	if idx := strings.IndexAny(rule, " \t"); idx > 0 {
		wantMethod := strings.ToUpper(strings.TrimSpace(rule[:idx]))
		rule = strings.TrimSpace(rule[idx+1:])
		// 路由以 ALL 声明时对任何方法生效，因此方法规则也应命中。
		if wantMethod != "" && wantMethod != "ALL" &&
			!strings.EqualFold(wantMethod, method) && !strings.EqualFold(method, "ALL") {
			return false
		}
	}
	return matchRoutePath(rule, path)
}

// matchRoutePath 按路径段匹配，避免 `/users` 误命中 `/usersettings`。
func matchRoutePath(pattern, path string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}
	if pattern == "*" || pattern == "**" || pattern == "/*" {
		return true
	}
	segments := splitRoutePath(pattern)
	actual := splitRoutePath(path)
	for i, segment := range segments {
		if segment == "*" || segment == "**" {
			return true
		}
		if i >= len(actual) {
			return false
		}
		if isPathParam(segment) {
			continue
		}
		if segment != actual[i] {
			return false
		}
	}
	return len(segments) == len(actual)
}

func isPathParam(segment string) bool {
	if strings.HasPrefix(segment, ":") && len(segment) > 1 {
		return true
	}
	return strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") && len(segment) > 2
}

func splitRoutePath(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

// DefaultMiddlewareConsumer 是 MiddlewareConsumer 的默认实现。
type DefaultMiddlewareConsumer struct {
	bindings []*middlewareBinding
	errs     []error
}

type middlewareBinding struct {
	middlewares   []MiddlewareFunc
	includeRoutes []string
	excludeRoutes []string
}

func (b *middlewareBinding) ForRoutes(routes ...string) MiddlewareRouteConfigurator {
	b.includeRoutes = append(b.includeRoutes, routes...)
	return b
}

func (b *middlewareBinding) Exclude(routes ...string) MiddlewareRouteConfigurator {
	b.excludeRoutes = append(b.excludeRoutes, routes...)
	return b
}

// NewMiddlewareConsumer 创建默认的 MiddlewareConsumer 实现。
func NewMiddlewareConsumer() *DefaultMiddlewareConsumer {
	return &DefaultMiddlewareConsumer{bindings: make([]*middlewareBinding, 0)}
}

// Apply 追加一组中间件，并返回该组中间件的路由范围配置器。
//
// 参数与 UseMiddlewares 一致：支持接口实现、具名函数类型、裸函数与延迟构造器。
// 类型不支持时错误会被记录下来，最终由 Err 返回（模块加载阶段会终止启动）。
func (c *DefaultMiddlewareConsumer) Apply(middlewares ...any) MiddlewareRouteConfigurator {
	binding := &middlewareBinding{}
	if c == nil {
		return binding
	}
	if len(middlewares) == 0 {
		c.errs = append(c.errs, errors.New("aop: MiddlewareConsumer.Apply requires at least one middleware"))
		return binding
	}
	converted, err := CoerceMiddlewares(middlewares)
	if err != nil {
		c.errs = append(c.errs, err)
		return binding
	}
	binding.middlewares = converted
	c.bindings = append(c.bindings, binding)
	return binding
}

// Bindings 返回当前 consumer 收集到的中间件绑定（深拷贝快照）。
func (c *DefaultMiddlewareConsumer) Bindings() []ScopedMiddleware {
	if c == nil || len(c.bindings) == 0 {
		return nil
	}
	out := make([]ScopedMiddleware, 0, len(c.bindings))
	for _, binding := range c.bindings {
		out = append(out, ScopedMiddleware{
			Middlewares:   append([]MiddlewareFunc{}, binding.middlewares...),
			IncludeRoutes: append([]string{}, binding.includeRoutes...),
			ExcludeRoutes: append([]string{}, binding.excludeRoutes...),
		})
	}
	return out
}

// Err 返回 Apply 期间累积的配置错误。
func (c *DefaultMiddlewareConsumer) Err() error {
	if c == nil || len(c.errs) == 0 {
		return nil
	}
	return errors.Join(c.errs...)
}
