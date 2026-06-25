// plugin.go 实现 MQ 协议的编译插件。
package mq

import (
	"fmt"
	"reflect"

	mqbinding "github.com/sao-lang/lania-g/protocol/mq/v3/binding"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	mqprotocol "github.com/sao-lang/lania-g/protocol/mq/v3/protocol"
)

// AdapterID 是 MQ 协议插件与 adapter 的统一标识。
const AdapterID = "mq"

// Plugin 是 MQ 协议的编译插件，用于把订阅声明编译为运行期路由。
type Plugin struct{}

// NewPlugin 创建一个 MQ 协议插件实例。
func NewPlugin() compiler.ProtocolPlugin { return &Plugin{} }

// ID 返回插件 ID。
func (p *Plugin) ID() string { return AdapterID }

// Protocol 返回该插件负责的协议类型（MQ）。
func (p *Plugin) Protocol() runtime.Protocol { return mqprotocol.Protocol }

// Register 注册 MQ 协议需要的默认 binding（用于参数解析与注入）。
func (p *Plugin) Register(reg *registry.Registry) {
	mqbinding.RegisterDefaultsToRegistry(reg)
}

type scanResult struct {
	routes []*routeOwnership
}

// Scan 扫描 registry 中声明的 MQ 订阅，并解析其归属模块与容器信息。
//
// receiver 既可能挂在 controller，也可能挂在 provider/resolver 上，
// 因此 owner 解析同样需要覆盖三类模块槽位。
func (p *Plugin) Scan(moduleRef *module.ModuleRef, reg *registry.Registry) (any, error) {
	routes, err := compiler.ScanResolvedRegistryItems(AdapterID, moduleRef, reg,
		"mq plugin scan requires an explicit registry; pass the application registry instance or registry.Global() explicitly for compatibility",
		compiler.SnapshotOwnerOptions{
			Controllers: true,
			Resolvers:   true,
			Providers:   true,
		},
		"subscriptions",
		func(raw any) (*SubscriptionDefinition, bool) {
			def, ok := raw.(*SubscriptionDefinition)
			return def, ok && def != nil
		},
		func(def *SubscriptionDefinition) any { return def.Receiver },
		func(def *SubscriptionDefinition) string {
			return fmt.Sprintf("mq subscription %s/%s", def.Consumer, def.Topic)
		},
		func(def *SubscriptionDefinition) map[string]any {
			return map[string]any{
				"consumer": def.Consumer,
				"topic":    def.Topic,
			}
		},
		"ensure it belongs to exactly one module-owned receiver slot",
		func(def *SubscriptionDefinition, own compiler.ModuleOwner, token reflect.Type) *routeOwnership {
			return &routeOwnership{
				definition:    def,
				moduleKey:     own.ModuleKey,
				moduleType:    own.ModuleType,
				receiverToken: token,
				container:     own.Container,
			}
		},
	)
	if err != nil {
		return nil, err
	}
	return &scanResult{routes: routes}, nil
}

// Compile 把 Scan 阶段收集到的订阅声明编译为可执行的运行期路由集合。
// MQ 这里也走通用 `CompileSimpleProtocol` 外壳，冲突键为 `consumer/topic`。
func (p *Plugin) Compile(scan any, reg *registry.Registry, global registry.GlobalAOPRegistration) (*compiler.CompiledProtocol, error) {
	return compiler.CompileSimpleProtocol(scan, mqprotocol.Protocol, AdapterID, "invalid scan result for mq plugin",
		func(scan any) ([]*routeOwnership, bool) {
			result, ok := scan.(*scanResult)
			if !ok || result == nil {
				return nil, false
			}
			return result.routes, true
		},
		global,
		func(owned *routeOwnership, global registry.GlobalAOPRegistration) (*compiler.CompiledRoute[*routeOwnership], error) {
			h, routeKey, err := compileSubscription(owned, global)
			if err != nil {
				return nil, err
			}
			return &compiler.CompiledRoute[*routeOwnership]{
				Item:      owned,
				Handler:   h,
				RouteKey:  routeKey,
				Container: owned.container,
			}, nil
		},
		func(owned *routeOwnership) string {
			return fmt.Sprintf("duplicate mq route %s/%s", owned.definition.Consumer, owned.definition.Topic)
		},
	)
}

// compileSubscription 把单条 MQ 订阅声明编译成 runtime.Handler。
// 当前版本不在 adapter 层追加本地 AOPSources，先只保留全局 AOP 注入点。
func compileSubscription(item *routeOwnership, global registry.GlobalAOPRegistration) (*runtime.Handler, string, error) {
	def := item.definition
	if def == nil || def.Receiver == nil || def.HandlerName == "" {
		return nil, "", fmt.Errorf("invalid mq subscription declaration: consumer=%s topic=%s", def.Consumer, def.Topic)
	}

	h, err := runtime.NewHandlerByToken(item.receiverToken, def.HandlerName)
	if err != nil {
		return nil, "", err
	}

	plan := compiler.CompileAOPPlan(global, compiler.AOPSources{})
	h.Meta.CompiledAOP = &plan

	// MQ 参数名依赖 DSL 显式绑定，而不是从消息体 schema 自动推断。
	h.Meta.ParamPlans = compiler.BuildBoundParamPlans(h.Meta.ParamTypes, def.ParamBindings)

	routeKey := runtime.BuildRouteKey(mqprotocol.Protocol, def.Topic, def.Consumer)
	h.Meta.RouteKey = routeKey
	h.Meta.Protocol = mqprotocol.Protocol
	h.Meta.ModuleKey = item.moduleKey

	return h, routeKey, nil
}

var _ compiler.ProtocolPlugin = (*Plugin)(nil)
