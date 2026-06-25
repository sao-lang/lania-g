package aop

// NestMiddleware 定义中间件的核心行为：在 handler 执行前后包裹一层通用逻辑。
type NestMiddleware interface {
	Use(ctx *ExecutionContext, next func() error) error
}

// Middleware 是框架对“中间件”对象的统一抽象。
type Middleware interface {
	NestMiddleware
}

// MiddlewareConstructor 用于延迟创建 Middleware 实例。
type MiddlewareConstructor func() Middleware

// MiddlewareFunc 是 Middleware 的函数式写法。
type MiddlewareFunc func(ctx *ExecutionContext, next func() error) error

// Use 让 MiddlewareFunc 适配 Middleware 接口。
//
// 约定：
// - next() 表示继续执行后续 middleware/最终 handler
// - middleware 可以选择不调用 next() 以中断链路（通常返回一个 error 表示中断原因）
func (f MiddlewareFunc) Use(ctx *ExecutionContext, next func() error) error {
	return f(ctx, next)
}

// MiddlewareConfigurable 表示某个模块或配置对象可以声明式注册中间件。
type MiddlewareConfigurable interface {
	Configure(consumer MiddlewareConsumer)
}

// MiddlewareConsumer 用于收集中间件以及它们的生效路由范围。
type MiddlewareConsumer interface {
	Apply(middlewares ...Middleware) MiddlewareConsumer
	ForRoutes(routes ...string) MiddlewareConsumer
	Exclude(routes ...string) MiddlewareConsumer
}

// RouteInfo 描述一条路由的基础信息。
type RouteInfo struct {
	Path   string
	Method string
}

// DefaultMiddlewareConsumer 是 MiddlewareConsumer 的默认实现。
type DefaultMiddlewareConsumer struct {
	middlewares     []Middleware
	includeRoutes   []string
	excludeRoutes   []string
}

// NewMiddlewareConsumer 创建默认的 MiddlewareConsumer 实现。
//
// 该 consumer 用于“声明式”配置中间件应用范围：
// - Apply: 追加中间件
// - ForRoutes: 仅对指定路由生效（白名单）
// - Exclude: 对指定路由不生效（黑名单）
func NewMiddlewareConsumer() *DefaultMiddlewareConsumer {
	return &DefaultMiddlewareConsumer{
		middlewares:   make([]Middleware, 0),
		includeRoutes: make([]string, 0),
		excludeRoutes: make([]string, 0),
	}
}

// Apply 追加一组中间件到当前 consumer。
//
// 注意：这里不做去重，保持配置顺序；实际执行顺序由上层 pipeline/编译期计划决定。
func (c *DefaultMiddlewareConsumer) Apply(middlewares ...Middleware) MiddlewareConsumer {
	c.middlewares = append(c.middlewares, middlewares...)
	return c
}

// ForRoutes 追加“白名单路由”列表：仅当 path 命中 includeRoutes 时才应用这些中间件。
func (c *DefaultMiddlewareConsumer) ForRoutes(routes ...string) MiddlewareConsumer {
	c.includeRoutes = append(c.includeRoutes, routes...)
	return c
}

// Exclude 追加“黑名单路由”列表：当 path 命中 excludeRoutes 时不应用这些中间件。
func (c *DefaultMiddlewareConsumer) Exclude(routes ...string) MiddlewareConsumer {
	c.excludeRoutes = append(c.excludeRoutes, routes...)
	return c
}

// GetMiddlewares 返回当前 consumer 收集到的中间件列表（原始顺序）。
func (c *DefaultMiddlewareConsumer) GetMiddlewares() []Middleware {
	return c.middlewares
}

// ShouldApply 判断给定 path 是否应该应用当前 consumer 中配置的中间件。
//
// 匹配规则（当前实现为“精确字符串匹配”）：
// - 若 path 命中 excludeRoutes，直接返回 false
// - 若 includeRoutes 为空，表示不做白名单限制，默认返回 true
// - 否则仅当 path 命中 includeRoutes 才返回 true
//
// 注意：这里不处理 path pattern（如 `/users/:id`），上层如果需要更复杂策略应自行扩展。
func (c *DefaultMiddlewareConsumer) ShouldApply(path string) bool {
	if len(c.excludeRoutes) > 0 {
		for _, exclude := range c.excludeRoutes {
			if path == exclude {
				return false
			}
		}
	}

	if len(c.includeRoutes) == 0 {
		return true
	}

	for _, include := range c.includeRoutes {
		if path == include {
			return true
		}
	}

	return false
}

// MiddlewareModule 表示模块本身支持声明式配置 middleware。
type MiddlewareModule interface {
	Configure(consumer MiddlewareConsumer)
}
