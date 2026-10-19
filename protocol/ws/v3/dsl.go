// dsl.go 提供 WS adapter 的声明式注册 DSL。
package ws

import (
	"fmt"
	"strings"

	coreadapter "github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// Controller 由需要声明 WS 事件处理器的 gateway 实现。
type Controller interface {
	Declare(*Routes)
}

// Routes 收集一个 gateway 声明的全部事件处理器。
//
// 它是 WS 唯一的声明入口：控制器在所属模块内完成注册，无需手动 Build。
type Routes struct {
	reg      *registry.Registry
	gateway  any
	current  *GatewayBuilder
	err      error
}

func newRoutes(reg *registry.Registry, gateway any) *Routes {
	return &Routes{reg: reg, gateway: gateway}
}

// Gateway 声明一个 namespace(prefix) 下的事件处理器。
func (r *Routes) Gateway(prefix string) *GatewayBuilder {
	if err := r.sealCurrent(); err != nil && r.err == nil {
		r.err = err
	}
	b := newGatewayBuilder(prefix, r.gateway, r)
	r.current = b
	return b
}

// Err 结束声明并返回累计错误。
func (r *Routes) Err() error {
	if r == nil {
		return nil
	}
	if err := r.sealCurrent(); err != nil && r.err == nil {
		r.err = err
	}
	return r.err
}

func (r *Routes) sealCurrent() error {
	if r == nil || r.current == nil {
		return nil
	}
	b := r.current
	r.current = nil
	return b.seal()
}

// GatewayBuilder 用于构建并注册某个 namespace 下的 gateway 事件处理器声明。
type GatewayBuilder struct {
	prefix         string
	gateway        any
	handlers       []*HandlerBuilder
	guards         []any
	interceptors   []any
	middlewares    []any
	pipes          []any
	paramPipes     map[int][]any
	filters        []any
	registry       *registry.Registry
	routes         *Routes
	err            error
}

// HandlerBuilder 用于构建单个 event 的处理器声明及其 AOP 配置。
type HandlerBuilder struct {
	gatewayBuilder *GatewayBuilder
	event          string
	handler        any
	methodName     string
	guards         []any
	interceptors   []any
	middlewares    []any
	pipes          []any
	paramPipes     map[int][]any
	filters        []any
}

// newGatewayBuilder 构建 GatewayBuilder，并绑定到给定 registry。
//
// builder 会收集 handlers 与 AOP 配置，最终在 Build/BuildE 时写入 registry 声明。
func newGatewayBuilder(prefix string, gateway any, routes *Routes) *GatewayBuilder {
	return &GatewayBuilder{
		prefix:     prefix,
		gateway:    gateway,
		handlers:   make([]*HandlerBuilder, 0),
		paramPipes: make(map[int][]any),
		routes:     routes,
		registry:   routes.reg,
	}
}

// UseParamPipes 为 gateway 下的所有 handler 追加“参数级 pipes”。
//
// 参数级 pipes 会在运行时对指定参数索引的值做 Transform（见 runtime.Executor.applyParamPipes）。
func (b *GatewayBuilder) UseParamPipes(paramIndex int, pipes ...any) *GatewayBuilder {
	if b.paramPipes[paramIndex] == nil {
		b.paramPipes[paramIndex] = make([]any, 0)
	}
	b.paramPipes[paramIndex] = append(b.paramPipes[paramIndex], pipes...)
	return b
}

// UseGuards 为 gateway 下的所有 handler 追加 guards。
func (b *GatewayBuilder) UseGuards(items ...any) *GatewayBuilder {
	b.guards = append(b.guards, items...)
	return b
}

// UseInterceptors 为 gateway 下的所有 handler 追加 interceptors。
func (b *GatewayBuilder) UseInterceptors(items ...any) *GatewayBuilder {
	b.interceptors = append(b.interceptors, items...)
	return b
}

// UseMiddlewares 为 gateway 下的所有 handler 追加 middlewares。
func (b *GatewayBuilder) UseMiddlewares(items ...any) *GatewayBuilder {
	b.middlewares = append(b.middlewares, items...)
	return b
}

// UsePipes 为 gateway 下的所有 handler 追加 pipes。
func (b *GatewayBuilder) UsePipes(items ...any) *GatewayBuilder {
	b.pipes = append(b.pipes, items...)
	return b
}

// UseFilters 为 gateway 下的所有 handler 追加 exception filters。
func (b *GatewayBuilder) UseFilters(items ...any) *GatewayBuilder {
	b.filters = append(b.filters, items...)
	return b
}

// On 声明一个事件处理器：event -> handler 方法。
//
// handler 参数通常传入 gateway 的“绑定方法”（例如 `gw.HandlePing`），builder 会尝试推导 methodName。
func (b *GatewayBuilder) On(event string, handler any) *HandlerBuilder {
	h := &HandlerBuilder{
		gatewayBuilder: b,
		event:          event,
		handler:        handler,
		methodName:     coreadapter.FindMethodName(b.gateway, handler),
		guards:         make([]any, 0),
		interceptors:   make([]any, 0),
		middlewares:    make([]any, 0),
		pipes:          make([]any, 0),
		paramPipes:     make(map[int][]any),
		filters:        make([]any, 0),
	}
	b.handlers = append(b.handlers, h)
	return h
}

// seal 把当前 gateway 的 handler 声明写入 registry。
func (b *GatewayBuilder) seal() error {
	if err := b.validate(); err != nil {
		b.err = err
		return err
	}
	defs := make([]*HandlerDefinition, 0, len(b.handlers))
	for _, h := range b.handlers {
		defs = append(defs, h.build(b))
	}
	items := make([]any, 0, len(defs))
	for _, def := range defs {
		items = append(items, def)
	}
	if b.registry != nil {
		b.registry.RegisterDecl(AdapterID, "handlers", items...)
	}
	return nil
}

// Err 返回最近一次 BuildE/validate 产生的错误（若无则为 nil）。
func (b *GatewayBuilder) Err() error { return b.err }

// UseGuards 为单个 handler 追加 guards（仅对该事件生效）。
func (hb *HandlerBuilder) UseGuards(items ...any) *HandlerBuilder {
	hb.guards = append(hb.guards, items...)
	return hb
}

// UseInterceptors 为单个 handler 追加 interceptors（仅对该事件生效）。
func (hb *HandlerBuilder) UseInterceptors(items ...any) *HandlerBuilder {
	hb.interceptors = append(hb.interceptors, items...)
	return hb
}

// UseMiddlewares 为单个 handler 追加 middlewares（仅对该事件生效）。
func (hb *HandlerBuilder) UseMiddlewares(items ...any) *HandlerBuilder {
	hb.middlewares = append(hb.middlewares, items...)
	return hb
}

// UsePipes 为单个 handler 追加 pipes（仅对该事件生效）。
func (hb *HandlerBuilder) UsePipes(items ...any) *HandlerBuilder {
	hb.pipes = append(hb.pipes, items...)
	return hb
}

// UseParamPipes 为单个 handler 追加参数级 pipes（仅对该事件生效）。
func (hb *HandlerBuilder) UseParamPipes(paramIndex int, items ...any) *HandlerBuilder {
	if hb.paramPipes[paramIndex] == nil {
		hb.paramPipes[paramIndex] = make([]any, 0)
	}
	hb.paramPipes[paramIndex] = append(hb.paramPipes[paramIndex], items...)
	return hb
}

// UseFilters 为单个 handler 追加 exception filters（仅对该事件生效）。
func (hb *HandlerBuilder) UseFilters(items ...any) *HandlerBuilder {
	hb.filters = append(hb.filters, items...)
	return hb
}

// build 生成一个 HandlerDefinition，并将 gateway 级别 AOP 与 handler 级别 AOP 合并。
//
// 合并规则：gateway 级别在前，handler 级别在后（更靠近 handler）。
func (hb *HandlerBuilder) build(gb *GatewayBuilder) *HandlerDefinition {
	return &HandlerDefinition{
		Prefix:       gb.prefix,
		Event:        hb.event,
		Gateway:      gb.gateway,
		MethodName:   hb.methodName,
		Guards:       append(append([]any{}, gb.guards...), hb.guards...),
		Interceptors: append(append([]any{}, gb.interceptors...), hb.interceptors...),
		Middlewares:  append(append([]any{}, gb.middlewares...), hb.middlewares...),
		Pipes:        append(append([]any{}, gb.pipes...), hb.pipes...),
		ParamPipes:   coreadapter.MergeParamPipes(gb.paramPipes, hb.paramPipes),
		Filters:      append(append([]any{}, gb.filters...), hb.filters...),
	}
}

// On 允许从 handler builder 继续声明下一个事件处理器（与 gRPC/GraphQL DSL 对齐）。
//
// 这样可以写成：
//
//	ws.Gateway("/chat", gw).
//		On("ping", gw.Ping).
//		On("join", gw.Join).
//		Build()
func (hb *HandlerBuilder) On(event string, handler any) *HandlerBuilder {
	if hb == nil || hb.gatewayBuilder == nil {
		return nil
	}
	return hb.gatewayBuilder.On(event, handler)
}

// Err 代理到 GatewayBuilder.Err，便于链式调用时读取构建错误。
func (hb *HandlerBuilder) Err() error {
	if hb == nil || hb.gatewayBuilder == nil {
		return fmt.Errorf("ws handler builder is nil")
	}
	return hb.gatewayBuilder.Err()
}

// validate 校验 gateway builder 声明的基本合法性。
//
// 校验项：
// - gateway 不为空
// - 每个 handler builder 不为空、event 非空
// - handler 函数与 methodName 合法（methodName 能被推导）
func (b *GatewayBuilder) validate() error {
	if b.gateway == nil {
		return fmt.Errorf("ws gateway is nil")
	}
	for _, h := range b.handlers {
		if h == nil {
			return fmt.Errorf("ws handler builder is nil")
		}
		if strings.TrimSpace(h.event) == "" {
			return fmt.Errorf("ws event is required")
		}
		if h.handler == nil || strings.TrimSpace(h.methodName) == "" {
			return fmt.Errorf("invalid ws handler declaration for event %s", h.event)
		}
	}
	return nil
}
