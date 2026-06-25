// plugin.go 实现 Scheduler 协议的编译插件。
package scheduler

import (
	"fmt"
	"reflect"

	schedulerbinding "github.com/sao-lang/lania-g/protocol/scheduler/v3/binding"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	schedulerprotocol "github.com/sao-lang/lania-g/protocol/scheduler/v3/protocol"
)

// AdapterID 是 Scheduler 协议插件与 adapter 的统一标识。
const AdapterID = "scheduler"

// Plugin 是 scheduler 协议的编译插件。
type Plugin struct{}

// NewPlugin 创建一个 scheduler 协议插件。
func NewPlugin() compiler.ProtocolPlugin { return &Plugin{} }

// ID 返回插件标识。
func (p *Plugin) ID() string { return AdapterID }

// Protocol 返回插件对应的协议类型。
func (p *Plugin) Protocol() runtime.Protocol {
	return schedulerprotocol.Protocol
}

// Register 向 registry 注册 scheduler 默认 binding。
func (p *Plugin) Register(reg *registry.Registry) {
	schedulerbinding.RegisterDefaultsToRegistry(reg)
}

type scanResult struct {
	routes []*routeOwnership
}

// Scan 扫描 registry 中声明的任务，并解析其所属模块。
//
// scheduler job 的 receiver 可能来自 controller/provider/resolver，
// 因此这里同样复用通用 owner 解析 helper。
func (p *Plugin) Scan(moduleRef *module.ModuleRef, reg *registry.Registry) (any, error) {
	routes, err := compiler.ScanResolvedRegistryItems(AdapterID, moduleRef, reg,
		"scheduler plugin scan requires an explicit registry; pass the application registry instance or registry.Global() explicitly for compatibility",
		compiler.SnapshotOwnerOptions{
			Controllers: true,
			Resolvers:   true,
			Providers:   true,
		},
		"jobs",
		func(raw any) (*JobDefinition, bool) {
			def, ok := raw.(*JobDefinition)
			return def, ok && def != nil
		},
		func(def *JobDefinition) any { return def.Receiver },
		func(def *JobDefinition) string { return fmt.Sprintf("scheduler job %s", def.Name) },
		func(def *JobDefinition) map[string]any {
			return map[string]any{
				"job": def.Name,
			}
		},
		"ensure it belongs to exactly one module-owned receiver slot",
		func(def *JobDefinition, own compiler.ModuleOwner, token reflect.Type) *routeOwnership {
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

// Compile 把扫描结果编译为运行期可安装的 scheduler 协议产物。
// routeKey 维度采用 `triggerKind/name`，以区分同名任务在不同触发类型下的声明。
func (p *Plugin) Compile(scan any, reg *registry.Registry, global registry.GlobalAOPRegistration) (*compiler.CompiledProtocol, error) {
	return compiler.CompileSimpleProtocol(scan, schedulerprotocol.Protocol, AdapterID, "invalid scan result for scheduler plugin",
		func(scan any) ([]*routeOwnership, bool) {
			result, ok := scan.(*scanResult)
			if !ok || result == nil {
				return nil, false
			}
			return result.routes, true
		},
		global,
		func(owned *routeOwnership, global registry.GlobalAOPRegistration) (*compiler.CompiledRoute[*routeOwnership], error) {
			h, routeKey, err := compileJob(owned, global)
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
			return fmt.Sprintf("duplicate scheduler route %s/%s", owned.definition.TriggerKind, owned.definition.Name)
		},
	)
}

// compileJob 把单个任务声明编译成 runtime.Handler。
// Scheduler 目前不在 adapter 层叠加本地 AOPSources，只挂接全局 AOP。
func compileJob(item *routeOwnership, global registry.GlobalAOPRegistration) (*runtime.Handler, string, error) {
	def := item.definition
	if def == nil || def.Receiver == nil || def.HandlerName == "" {
		return nil, "", fmt.Errorf("invalid scheduler job declaration: %s", def.Name)
	}
	h, err := runtime.NewHandlerByToken(item.receiverToken, def.HandlerName)
	if err != nil {
		return nil, "", err
	}
	plan := compiler.CompileAOPPlan(global, compiler.AOPSources{})
	h.Meta.CompiledAOP = &plan
	// 参数绑定名由 DSL 预先记录，编译阶段只负责转成 runtime.ParamPlan。
	h.Meta.ParamPlans = compiler.BuildBoundParamPlans(h.Meta.ParamTypes, def.ParamBindings)
	routeKey := runtime.BuildRouteKey(schedulerprotocol.Protocol, string(def.TriggerKind), def.Name)
	h.Meta.RouteKey = routeKey
	h.Meta.Protocol = schedulerprotocol.Protocol
	h.Meta.ModuleKey = item.moduleKey
	return h, routeKey, nil
}

var _ compiler.ProtocolPlugin = (*Plugin)(nil)
