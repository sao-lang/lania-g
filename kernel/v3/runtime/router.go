// router.go 实现 runtime 的路由注册与匹配层。
//
// Runtime 只认 `(protocol, method, path)` 这组三元组；
// 更复杂的协议匹配规则，例如 HTTP 模板路由，则通过 RouteMatcher 作为扩展点接进来。
package runtime

import (
	"fmt"
	"maps"
	"strings"
	"sync"

	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
)

// RouteKey 是 runtime 内部用于唯一标识一条路由的三元组。
type RouteKey struct {
	Protocol Protocol
	Method   string
	Path     string
}

// String 将 RouteKey 序列化为标准字符串格式：`{protocol}:{method}:{path}`。
//
// 该字符串是 runtime 内部的“主键”，用于 routes map 的索引与日志/诊断输出。
func (rk RouteKey) String() string {
	return fmt.Sprintf("%s:%s:%s", rk.Protocol, rk.Method, rk.Path)
}

// ParseRouteKey 解析标准 RouteKey 字符串：`{protocol}:{method}:{path}`。
//
// 注意 path 允许包含 `:`（例如某些协议的路由名），因此 SplitN(..., 3)。
func ParseRouteKey(key string) (RouteKey, error) {
	protocol, rest, ok := strings.Cut(key, ":")
	if !ok {
		return RouteKey{}, ErrInvalidProtocol
	}
	method, path, ok := strings.Cut(rest, ":")
	if !ok {
		return RouteKey{}, ErrInvalidProtocol
	}
	return RouteKey{
		Protocol: Protocol(protocol),
		Method:   method,
		Path:     path,
	}, nil
}

// BuildRouteKey 构造 routeKey 字符串：`{protocol}:{method}:{path}`。
//
// 该函数是 Router 进行精确匹配时的标准入口；协议适配器注册路由时也应使用同样的规则。
func BuildRouteKey(protocol Protocol, method, path string) string {
	return fmt.Sprintf("%s:%s:%s", protocol, method, path)
}

// Router 维护 routeKey -> handler 的映射，并支持按协议安装 matcher 做非精确匹配（如 HTTP path params）。
// 它刻意保持简单：精确匹配是默认主路径，matcher 只作为补充能力。
type Router struct {
	routes   map[string]*Handler
	matchers map[Protocol]RouteMatcher
	mu       sync.RWMutex
}

// NewRouter 创建一个空 Router。
//
// 默认仅支持精确 routeKey 匹配；协议可通过 SetMatcher 安装自定义 matcher（例如 HTTP 模板路由）。
func NewRouter() *Router {
	return &Router{
		routes:   make(map[string]*Handler),
		matchers: make(map[Protocol]RouteMatcher),
	}
}

// RouteMatcher 由具体协议实现，用于处理非精确匹配场景。
// 典型例子是 HTTP 的模板路由、动态路径参数等。
type RouteMatcher interface {
	Match(ctx *HandlerContext) (*Handler, map[string]string)
}

// SetMatcher 为指定协议设置 RouteMatcher。
//
// matcher 用于“非精确匹配”（例如 HTTP `/users/:id`）；Router.Match 会优先走精确匹配，失败后才调用 matcher。
// 传入 nil 会被忽略。
func (r *Router) SetMatcher(protocol Protocol, m RouteMatcher) {
	if m == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.matchers == nil {
		r.matchers = make(map[Protocol]RouteMatcher)
	}
	r.matchers[protocol] = m
}

// Register 注册一个确定的 routeKey。
// 精确匹配永远优先于 matcher（避免 matcher 覆盖已注册的静态路由）。
func (r *Router) Register(routeKey string, handler *Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.routes[routeKey] = handler
	if rk, err := ParseRouteKey(routeKey); err == nil {
		handler.Meta.RouteKey = routeKey
		handler.Meta.Protocol = rk.Protocol
	}
}

// Match 根据 ctx（协议/方法/路径）匹配到 Handler，并返回路由参数（如 path params）。
//
// 匹配顺序：
// 1) 精确匹配：BuildRouteKey(protocol, method, path) 直接命中 routes map
// 2) 协议 matcher：由协议适配器实现（例如 HTTP 的模板树匹配）
//
// 返回值：
// - handler：匹配到的处理器；未匹配时为 nil
// - params：matcher 解析出的参数（例如 `{id: "123"}`）；精确匹配时为 nil
// - err：未匹配到路由时为 ErrRouteNotFound
func (r *Router) Match(ctx *HandlerContext) (*Handler, map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	routeKey := BuildRouteKey(ctx.Protocol, ctx.Request.Method, ctx.Request.Path)

	// 1) 精确匹配：最快路径，也是静态路由的最高优先级。
	if handler, ok := r.routes[routeKey]; ok {
		return handler, nil, nil
	}

	// 2) matcher 匹配：由具体协议实现（如 HTTP 的 path template 匹配）。
	// 只有精确匹配失败时才进入，避免动态规则吞掉静态路由。
	if matcher := r.matchers[ctx.Protocol]; matcher != nil {
		handler, params := matcher.Match(ctx)
		if handler != nil {
			return handler, params, nil
		}
	}

	return nil, nil, ErrRouteNotFound
}

// InstallCompiledProtocol 安装编译期产物生成的 matcher/routes。
//
// 语义：
// - 如果 routes 中存在 routeKey 冲突，直接报错（避免静默覆盖导致不可预期行为）
// - 安装完成后会为 handler.Meta 填充 Protocol/RouteKey
func (r *Router) InstallCompiledProtocol(protocol Protocol, matcher RouteMatcher, routes map[string]*Handler) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if matcher != nil {
		if r.matchers == nil {
			r.matchers = make(map[Protocol]RouteMatcher)
		}
		r.matchers[protocol] = matcher
	}
	if routes == nil {
		return nil
	}
	if r.routes == nil {
		r.routes = make(map[string]*Handler, len(routes))
	}
	for k, v := range routes {
		if _, exists := r.routes[k]; exists {
			return &kerrors.KernelError{
				Kind:     kerrors.KindExecution,
				Protocol: string(protocol),
				RouteKey: k,
				Message:  fmt.Sprintf("route conflict detected for %s (duplicate routeKey during router install)", k),
				Meta: map[string]interface{}{
					"stage":  "install",
					"reason": "duplicate routeKey during router install",
				},
			}
		}
		r.routes[k] = v
		if rk, err := ParseRouteKey(k); err == nil && v != nil {
			v.Meta.RouteKey = k
			v.Meta.Protocol = rk.Protocol
		}
	}
	return nil
}

// Get 按 routeKey 获取已注册的 handler（只做精确查找，不触发 matcher）。
func (r *Router) Get(routeKey string) (*Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	handler, ok := r.routes[routeKey]
	return handler, ok
}

// Remove 删除一个 routeKey 对应的 handler。
//
// 注意：只影响精确匹配表；如果协议 matcher 内部也维护结构（如路由树），需要由对应协议自行更新。
func (r *Router) Remove(routeKey string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.routes, routeKey)
}

// AllRoutes 返回当前所有已注册 routes 的快照（浅拷贝 map）。
//
// 说明：
// - 返回的是新 map，避免调用方修改内部状态
// - 但 map value（*Handler）仍是同一指针，属于共享对象
func (r *Router) AllRoutes() map[string]*Handler {
	r.mu.RLock()
	defer r.mu.RUnlock()

	routes := make(map[string]*Handler, len(r.routes))
	maps.Copy(routes, r.routes)
	return routes
}
