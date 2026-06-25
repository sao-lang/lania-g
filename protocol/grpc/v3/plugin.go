// plugin.go 实现 gRPC 协议的编译插件。
package grpc

import (
	"fmt"
	"reflect"

	grpcbinding "github.com/sao-lang/lania-g/protocol/grpc/v3/binding"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	grpcprotocol "github.com/sao-lang/lania-g/protocol/grpc/v3/protocol"
)

// AdapterID 是 gRPC 协议插件与 adapter 的统一标识。
const AdapterID = "grpc"

// Plugin 是 gRPC 协议的编译插件。
type Plugin struct{}

// NewPlugin 创建一个 gRPC 协议插件。
func NewPlugin() compiler.ProtocolPlugin { return &Plugin{} }

// ID 返回插件标识。
func (p *Plugin) ID() string { return AdapterID }

// Protocol 返回插件对应的协议类型。
func (p *Plugin) Protocol() runtime.Protocol { return grpcprotocol.Protocol }

// Register 向 registry 注册 gRPC 默认 binding。
func (p *Plugin) Register(reg *registry.Registry) {
	grpcbinding.RegisterDefaultsToRegistry(reg)
}

type methodOwnership struct {
	definition    *MethodDefinition
	moduleKey     string
	moduleType    reflect.Type
	container     *di.Container
	// receiverToken 是编译期固定下来的 DI token。
	// 运行时真正调用 handler 时，会再通过这个 token 到 request-scope 容器里解析实例。
	receiverToken reflect.Type
}

type scanResult struct {
	methods []*methodOwnership
}

// Scan 扫描 registry 中声明的 gRPC 方法，并解析其所属模块。
//
// gRPC method declaration 的 receiver 既可能是 controller，也可能是 provider/resolver，
// 所以 owner 解析时把三类模块槽位都纳入候选。
func (p *Plugin) Scan(moduleRef *module.ModuleRef, reg *registry.Registry) (any, error) {
	methods, err := compiler.ScanResolvedRegistryItems(AdapterID, moduleRef, reg,
		"grpc plugin scan requires an explicit registry; pass the application registry instance or registry.Global() explicitly for compatibility",
		compiler.SnapshotOwnerOptions{
			Controllers: true,
			Resolvers:   true,
			Providers:   true,
		},
		"methods",
		// registry 里仍然存的是统一的 MethodDefinition，
		// plugin 这里只负责把它们过滤出来并补齐 owner/container 信息。
		func(raw any) (*MethodDefinition, bool) {
			def, ok := raw.(*MethodDefinition)
			return def, ok && def != nil
		},
		func(def *MethodDefinition) any { return def.Receiver },
		func(def *MethodDefinition) string { return fmt.Sprintf("grpc method %s/%s", def.Service, def.Method) },
		func(def *MethodDefinition) map[string]any {
			return map[string]any{
				"service": def.Service,
				"method":  def.Method,
			}
		},
		"ensure it belongs to exactly one module-owned receiver slot",
		func(def *MethodDefinition, own compiler.ModuleOwner, token reflect.Type) *methodOwnership {
			return &methodOwnership{
				definition:    def,
				moduleKey:     own.ModuleKey,
				moduleType:    own.ModuleType,
				container:     own.Container,
				receiverToken: token,
			}
		},
	)
	if err != nil {
		return nil, err
	}
	return &scanResult{methods: methods}, nil
}

// Compile 把扫描结果编译为运行期可安装的 gRPC 协议产物。
// gRPC 这里可以完全复用通用 `CompileSimpleProtocol` 外壳，
// 特化逻辑只保留在单条 method 的编译函数里。
func (p *Plugin) Compile(scan any, reg *registry.Registry, global registry.GlobalAOPRegistration) (*compiler.CompiledProtocol, error) {
	return compiler.CompileSimpleProtocol(scan, grpcprotocol.Protocol, AdapterID, "invalid scan result for grpc plugin",
		func(scan any) ([]*methodOwnership, bool) {
			result, ok := scan.(*scanResult)
			if !ok || result == nil {
				return nil, false
			}
			return result.methods, true
		},
		global,
		func(owned *methodOwnership, global registry.GlobalAOPRegistration) (*compiler.CompiledRoute[*methodOwnership], error) {
			h, routeKey, err := compileMethod(owned, global)
			if err != nil {
				return nil, err
			}
			return &compiler.CompiledRoute[*methodOwnership]{
				Item:      owned,
				Handler:   h,
				RouteKey:  routeKey,
				Container: owned.container,
			}, nil
		},
		func(owned *methodOwnership) string {
			return fmt.Sprintf("duplicate grpc route %s/%s", owned.definition.Service, owned.definition.Method)
		},
	)
}

// compileMethod 负责把单条 gRPC 声明编译成 runtime.Handler。
// 它的特化点主要是：
// - 通过 `ParamBindings` 给 header/message 等参数补 BindingName
// - 用 `service/method` 生成 routeKey
func compileMethod(item *methodOwnership, global registry.GlobalAOPRegistration) (*runtime.Handler, string, error) {
	def := item.definition
	if def == nil {
		return nil, "", fmt.Errorf("nil grpc method definition")
	}
	if def.Receiver == nil || def.HandlerName == "" {
		return nil, "", fmt.Errorf("invalid grpc method declaration: %s/%s", def.Service, def.Method)
	}

	h, err := runtime.NewHandlerByToken(item.receiverToken, def.HandlerName)
	if err != nil {
		return nil, "", err
	}
	def.Mode = def.Mode.Normalize()
	// 在 handler 已经构造完成、参数和返回值元信息都可用之后，再做一次模式校验，
	// 这样可以直接基于真实签名给出更准确的错误信息。
	if err := validateMethodDefinition(def, h); err != nil {
		return nil, "", err
	}

	// gRPC adapter 没有单独发明一套 AOP 机制，而是继续复用统一编译器生成的执行计划。
	plan := compiler.CompileAOPPlan(global, compiler.AOPSources{
		Middlewares:  def.Middlewares,
		Guards:       def.Guards,
		Interceptors: def.Interceptors,
		Pipes:        def.Pipes,
		ParamPipes:   def.ParamPipes,
		Filters:      def.Filters,
	})
	h.Meta.CompiledAOP = &plan

	// gRPC 参数名通常不是从函数签名里猜，而是由 DSL 显式记录到 ParamBindings。
	h.Meta.ParamPlans = compiler.BuildBoundParamPlans(h.Meta.ParamTypes, def.ParamBindings)

	// routeKey 仍沿用统一 runtime 编码格式，后续 router、诊断、request-scope container 选择都依赖它。
	routeKey := runtime.BuildRouteKey(grpcprotocol.Protocol, def.Method, def.Service)
	h.Meta.RouteKey = routeKey
	h.Meta.Protocol = grpcprotocol.Protocol
	h.Meta.ModuleKey = item.moduleKey

	return h, routeKey, nil
}
