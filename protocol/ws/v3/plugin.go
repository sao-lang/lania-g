// plugin.go 实现 WS 协议的编译插件。
package ws

import (
	"fmt"
	"reflect"

	wsbinding "github.com/sao-lang/lania-g/protocol/ws/v3/binding"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	wsprotocol "github.com/sao-lang/lania-g/protocol/ws/v3/protocol"
)

// Plugin 是 WS 协议的编译插件，用于把 WS handler 声明编译为运行期路由。
type Plugin struct{}

// NewPlugin 创建 WS 协议的编译插件（ProtocolPlugin）。
//
// WS adapter 通过 compiler 插件体系把 DSL 声明编译为 runtime.Router 可安装的 routes。
func NewPlugin() compiler.ProtocolPlugin { return &Plugin{} }

// ID 返回插件 ID（用于 registry 按 pluginID 分组存储声明）。
func (p *Plugin) ID() string { return AdapterID }

// Protocol 返回该插件对应的协议标识（wsprotocol.Protocol）。
func (p *Plugin) Protocol() runtime.Protocol { return wsprotocol.Protocol }

// Register 注册 WS 协议的默认绑定/声明。
//
// 这里主要把 binding/ws 的默认 binding resolvers 注册到 registry，
// 使得 WS handler 的入参可以从 socket/event/header/body 等上下文中解析。
func (p *Plugin) Register(reg *registry.Registry) {
	wsbinding.RegisterDefaultsToRegistry(reg)
}

type handlerOwnership struct {
	definition   *HandlerDefinition
	moduleKey    string
	moduleType   reflect.Type
	container    *di.Container
	gatewayToken reflect.Type
}

type scanResult struct {
	handlers []*handlerOwnership
}

// Scan 扫描 registry 中 WS handler 声明，并解析每个 gateway 的 owner 模块容器。
//
// 关键点：
// - gateway 可能出现在多个模块中（同类型多 owner），因此会用 compiler.BuildSnapshotOwnerIndex 做归属推导
// - 若缺失 owner 或 owner 歧义，会返回 KernelError(KindDI) 以便用户修复模块声明
func (p *Plugin) Scan(moduleRef *module.ModuleRef, reg *registry.Registry) (any, error) {
	handlers, err := compiler.ScanResolvedRegistryItems(AdapterID, moduleRef, reg,
		"ws plugin scan requires an explicit registry; pass the application registry instance or registry.Global() explicitly for compatibility",
		compiler.SnapshotOwnerOptions{
			Controllers: true,
			Resolvers:   true,
		},
		"handlers",
		func(raw any) (*HandlerDefinition, bool) {
			def, ok := raw.(*HandlerDefinition)
			return def, ok && def != nil
		},
		func(def *HandlerDefinition) any { return def.Gateway },
		func(def *HandlerDefinition) string { return fmt.Sprintf("ws handler %s", def.Event) },
		func(def *HandlerDefinition) map[string]any {
			return map[string]any{
				"event": def.Event,
			}
		},
		"",
		func(def *HandlerDefinition, own compiler.ModuleOwner, token reflect.Type) *handlerOwnership {
			return &handlerOwnership{
				definition:   def,
				moduleKey:    own.ModuleKey,
				moduleType:   own.ModuleType,
				container:    own.Container,
				gatewayToken: token,
			}
		},
	)
	if err != nil {
		return nil, err
	}
	return &scanResult{handlers: handlers}, nil
}

// Compile 将 Scan 的结果编译为可安装到 runtime 的 CompiledProtocol。
//
// 编译产物：
// - Routes：routeKey -> *runtime.Handler（Token 模式 handler，receiver 由 DI 解析）
// - RouteContainers：routeKey -> module container（供 runtime.Executor 为不同路由选择容器）
// - Install：安装函数（把 routes 注册进 Router；WS 不需要像 HTTP 那样额外构造 matcher）
func (p *Plugin) Compile(scan any, reg *registry.Registry, global registry.GlobalAOPRegistration) (*compiler.CompiledProtocol, error) {
	return compiler.CompileSimpleProtocol(scan, wsprotocol.Protocol, AdapterID, "invalid scan result for ws plugin",
		func(scan any) ([]*handlerOwnership, bool) {
			result, ok := scan.(*scanResult)
			if !ok || result == nil {
				return nil, false
			}
			return result.handlers, true
		},
		global,
		func(owned *handlerOwnership, global registry.GlobalAOPRegistration) (*compiler.CompiledRoute[*handlerOwnership], error) {
			h, routeKey, err := compileHandler(owned, global)
			if err != nil {
				return nil, err
			}
			return &compiler.CompiledRoute[*handlerOwnership]{
				Item:      owned,
				Handler:   h,
				RouteKey:  routeKey,
				Container: owned.container,
			}, nil
		},
		func(owned *handlerOwnership) string {
			return fmt.Sprintf("duplicate ws route %s@%s", owned.definition.Event, normalizeNamespace(owned.definition.Prefix))
		},
	)
}

// compileHandler 将单个 WS handler 声明编译为 runtime.Handler，并生成其 routeKey。
//
// 主要步骤：
// - 用 gatewayToken + methodName 构建 Token 模式 handler（每次请求从 DI 解析 receiver）
// - 将全局 AOP 与本地声明的 AOPSources 合并为编译期 AOPPlan，并写入 handler.Meta.CompiledAOP
// - 生成参数计划（ParamPlan），供运行时 binding 解析使用
// - 生成 routeKey：`ws:{event}:{namespace}`（namespace 来自 Prefix）
func compileHandler(item *handlerOwnership, global registry.GlobalAOPRegistration) (*runtime.Handler, string, error) {
	def := item.definition
	if def == nil {
		return nil, "", fmt.Errorf("nil ws handler definition")
	}
	if def.Gateway == nil || def.MethodName == "" {
		return nil, "", fmt.Errorf("invalid ws handler declaration: %s", def.Event)
	}

	h, err := runtime.NewHandlerByToken(item.gatewayToken, def.MethodName)
	if err != nil {
		return nil, "", err
	}

	plan := compiler.CompileAOPPlan(global, compiler.AOPSources{
		Middlewares:  def.Middlewares,
		Guards:       def.Guards,
		Interceptors: def.Interceptors,
		Pipes:        def.Pipes,
		ParamPipes:   def.ParamPipes,
		Filters:      def.Filters,
	})

	h.Meta.CompiledAOP = &plan
	h.Meta.ParamPlans = buildParamPlan(h)

	prefix := normalizeNamespace(def.Prefix)
	// WS 的 routeKey 由 event + namespace 决定，不像 HTTP 还要再拆 method/path 树。
	routeKey := runtime.BuildRouteKey(wsprotocol.Protocol, def.Event, prefix)
	h.Meta.RouteKey = routeKey
	h.Meta.Protocol = wsprotocol.Protocol
	h.Meta.ModuleKey = item.moduleKey

	return h, routeKey, nil
}

// buildParamPlan 生成一个最基础的参数计划：只记录参数索引与类型。
//
// WS handler 的参数名绑定通常由 binding/ws 在运行时根据上下文推导，
// 因此这里不写 BindingName。
func buildParamPlan(handler *runtime.Handler) []runtime.ParamPlan {
	out := make([]runtime.ParamPlan, 0, len(handler.Meta.ParamTypes))
	for i, typ := range handler.Meta.ParamTypes {
		out = append(out, runtime.ParamPlan{Index: i, Type: typ})
	}
	return out
}
