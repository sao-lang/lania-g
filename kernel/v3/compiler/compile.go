// compile.go 实现从 `moduleRef + registry + protocol plugins` 到 `CompiledApp` 的主编译流程。
//
// 它是 application 启动前最关键的一步：
// - 先让插件补默认声明
// - 再扫描声明并确定模块归属
// - 再编译成各协议可安装的产物
// - 最后把绑定系统、路由容器和诊断信息一起打包
package compiler

import (
	"fmt"
	"slices"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// CompiledApp 表示一次完整编译后的应用产物集合。
//
// 它是 compiler 交给 runtime 的中间结果，包含：
// - 运行时参数绑定系统
// - 各协议的编译产物与安装顺序
// - routeKey 到模块容器的映射
// - 编译阶段收集到的诊断信息
type CompiledApp struct {
	// BindingRegistry 是由 registry.Bindings 组装出来的运行时绑定系统（按“后注册优先”）。
	BindingRegistry *runtime.BindingRegistry
	// Protocols 是各协议插件编译后的产物（routes/matcher/install hook 等）。
	Protocols     map[runtime.Protocol]*CompiledProtocol
	ProtocolOrder []runtime.Protocol
	// RouteContainers 将 routeKey -> module container 映射出来，供 runtime.Executor 为不同路由选择 DI 容器。
	RouteContainers map[string]*di.Container
	GlobalAOP       registry.GlobalAOPRegistration
	Diagnostics     *CompileDiagnostics
}

// Compile 将 `moduleRef + registry` 中的声明编译为“可安装到 runtime 的产物”。
//
// 主流程：
// - 先让每个插件执行 Register，补齐默认 binding/声明
// - 再执行 Scan，确定协议声明与模块归属
// - 然后执行 Compile，生成协议专属的编译产物
// - 最后做跨协议 routeKey 冲突检测，并汇总诊断信息
//
// 推荐：显式传入实例级 registry。
// 兼容：若明确需要从全局 registry 读取声明，优先使用 `CompileCompat(...)`。
func Compile(moduleRef *module.ModuleRef, reg *registry.Registry, plugins ...ProtocolPlugin) (*CompiledApp, error) {
	if moduleRef == nil {
		return nil, recordCompileError(nil, fmt.Errorf("moduleRef is nil"))
	}
	orderedPlugins := stableProtocolPlugins(plugins)
	diag := &CompileDiagnostics{
		RegisteredPlugins: make([]string, 0, len(orderedPlugins)),
		Protocols:         make(map[runtime.Protocol]*ProtocolDiagnostics, len(orderedPlugins)),
		RegistryFallbacks: knownRegistryFallbacks(),
		RuntimeFallbacks:  knownRuntimeFallbacks(),
	}
	if reg == nil {
		return nil, recordCompileError(diag, fmt.Errorf("compiler.Compile requires an explicit registry; use compiler.CompileCompat(...) to read declarations from registry.Global()"))
	}
	if len(orderedPlugins) == 0 {
		return nil, recordCompileError(nil, fmt.Errorf("no protocol plugins registered"))
	}

	// 先让插件把默认声明写进 registry，再统一读取 bindings/decls。
	// 这样后续扫描时看到的是“插件默认值 + 用户声明”合并后的最终视图。
	for _, p := range orderedPlugins {
		if p == nil {
			continue
		}
		diag.RegisteredPlugins = append(diag.RegisteredPlugins, p.ID())
		diag.ProtocolOrder = append(diag.ProtocolOrder, p.Protocol())
		p.Register(reg)
	}

	// BindingRegistry 在编译期就固定下来，运行期只负责消费，不再重新推导默认绑定顺序。
	br := runtime.NewBindingRegistry()
	for _, resolver := range reg.GetBindings() {
		br.Register(resolver)
	}

	global := reg.GetGlobalAOP()
	diag.DeclarationCounts = reg.SnapshotDeclCounts()
	diag.BindingResolverCount = len(reg.GetBindings())
	diag.GlobalAOP = AOPDiagnostics{
		Middlewares:  len(global.Middlewares),
		Guards:       len(global.Guards),
		Interceptors: len(global.Interceptors),
		Pipes:        len(global.Pipes),
		Filters:      len(global.Filters),
	}

	compiled := &CompiledApp{
		BindingRegistry: br,
		Protocols:       make(map[runtime.Protocol]*CompiledProtocol),
		ProtocolOrder:   append([]runtime.Protocol{}, diag.ProtocolOrder...),
		RouteContainers: make(map[string]*di.Container),
		GlobalAOP:       global,
		Diagnostics:     diag,
	}

	installedRoutes := make(map[string]RouteConflict)

	for _, p := range orderedPlugins {
		if p == nil {
			continue
		}
		// 每个插件都遵循同样的 Scan -> Compile 两阶段，
		// 这样协议特化逻辑就能被约束在插件内部。
		scan, err := p.Scan(moduleRef, reg)
		if err != nil {
			return nil, recordCompileError(diag, err)
		}
		cp, err := p.Compile(scan, reg, global)
		if err != nil {
			return nil, recordCompileError(diag, err)
		}
		if cp == nil {
			continue
		}
		pd := &ProtocolDiagnostics{
			Protocol:         p.Protocol(),
			PluginID:         p.ID(),
			DeclarationKinds: cloneDeclarationCountMap(diag.DeclarationCounts[p.ID()]),
			DeclarationCount: countDecls(diag.DeclarationCounts[p.ID()]),
			RouteCount:       len(cp.Routes),
			RouteContainers:  len(cp.RouteContainers),
			OwnerModules:     summarizeRouteOwners(cp.Routes),
		}
		diag.Protocols[p.Protocol()] = pd
		for routeKey := range cp.Routes {
			if existing, ok := installedRoutes[routeKey]; ok {
				conflict := RouteConflict{
					RouteKey:       routeKey,
					Protocol:       p.Protocol(),
					PluginID:       p.ID(),
					ExistingRoute:  existing.RouteKey,
					ExistingPlugin: existing.PluginID,
					Reason:         "duplicate routeKey across compiled protocols",
				}
				diag.RouteConflicts = append(diag.RouteConflicts, conflict)
				return nil, recordCompileError(diag, NewRouteConflictError(conflict))
			}
			installedRoutes[routeKey] = RouteConflict{
				RouteKey: routeKey,
				Protocol: p.Protocol(),
				PluginID: p.ID(),
				Reason:   "registered",
			}
		}
		compiled.Protocols[p.Protocol()] = cp
		for k, v := range cp.RouteContainers {
			compiled.RouteContainers[k] = v
		}
	}

	return compiled, nil
}

// CompileCompat 显式保留“未传 registry 时从全局 registry 读取声明”的兼容语义。
func CompileCompat(moduleRef *module.ModuleRef, plugins ...ProtocolPlugin) (*CompiledApp, error) {
	return Compile(moduleRef, registry.Global(), plugins...)
}

func summarizeRouteOwners(routes map[string]*runtime.Handler) []ModuleRouteDiagnostics {
	if len(routes) == 0 {
		return nil
	}
	counts := make(map[string]int)
	routeKeysByModule := make(map[string][]string)
	for _, handler := range routes {
		if handler == nil || handler.Meta == nil {
			continue
		}
		moduleKey := handler.Meta.ModuleKey
		if moduleKey == "" {
			moduleKey = "<unknown>"
		}
		counts[moduleKey]++
		if handler.Meta.RouteKey != "" {
			routeKeysByModule[moduleKey] = append(routeKeysByModule[moduleKey], handler.Meta.RouteKey)
		}
	}
	if len(counts) == 0 {
		return nil
	}
	out := make([]ModuleRouteDiagnostics, 0, len(counts))
	for moduleKey, routes := range counts {
		routeKeys := append([]string{}, routeKeysByModule[moduleKey]...)
		slices.Sort(routeKeys)
		out = append(out, ModuleRouteDiagnostics{
			ModuleKey: moduleKey,
			Routes:    routes,
			RouteKeys: routeKeys,
		})
	}
	slices.SortStableFunc(out, func(a, b ModuleRouteDiagnostics) int {
		if a.Routes != b.Routes {
			if a.Routes > b.Routes {
				return -1
			}
			return 1
		}
		return compareStrings(a.ModuleKey, b.ModuleKey)
	})
	return out
}

func knownRuntimeFallbacks() []RuntimeFallbackDiagnostics {
	return []RuntimeFallbackDiagnostics{
		{
			Key:      "runtime_global_aop",
			Category: "compatibility",
			Summary:  "runtime.Pipeline still applies runtime-level global AOP when a handler has not been compiled with CompiledAOP",
		},
		{
			Key:      "binding_to_di",
			Category: "migratable",
			Summary:  "runtime.Executor falls back from protocol binding resolution to DI container lookup when no binding matches a parameter",
		},
		{
			Key:      "route_to_root_container",
			Category: "compatibility",
			Summary:  "runtime.Executor falls back to the root container when a compiled route does not provide a dedicated route container",
		},
	}
}

func knownRegistryFallbacks() []RegistryFallbackDiagnostics {
	return []RegistryFallbackDiagnostics{
		{
			Key:      "application_global_compat",
			Category: "compat",
			Summary:  "application.New(...) and application.NewCompat(...) build applications from registry.Global() as a compatibility path; new code should prefer application.NewWithOptions(..., Options{Registry: application.NewRegistry()})",
		},
		{
			Key:      "compile_registry_global_compat",
			Category: "compat",
			Summary:  "compiler.CompileCompat(...) reads declarations from registry.Global() as a compatibility path; new code should prefer compiler.Compile(moduleRef, reg, ...)",
		},
		{
			Key:      "graphql_schema_global_read",
			Category: "read-fallback",
			Summary:  "adapter/graphql.buildCompiledSchemaCompat(...) reads GraphQL config and resolver declarations from registry.Global() for compatibility helpers",
		},
		{
			Key:      "events_attach_global_read",
			Category: "read-fallback",
			Summary:  "integrations/events.AttachRegisteredHandlersCompat(...) reads handler declarations from registry.Global() for compatibility hooks/helpers",
		},
		{
			Key:      "swagger_http_registry_global_read",
			Category: "read-fallback",
			Summary:  "integrations/swagger.BuildFromHTTPRegistryCompat(...) reads HTTP route declarations from registry.Global() for compatibility helpers",
		},
	}
}

// Install 将编译产物安装到 runtime。
//
// 主要步骤：
// - 创建全新的 Router，并让各协议安装 matcher/routes，避免污染旧 router
// - 注入 BindingRegistry 与 RouteContainers，使 Executor 能解析参数并选择模块容器
// - 同步 global AOP 到 runtime，兼容某些 fallback 执行路径
func (c *CompiledApp) Install(rt *runtime.Runtime) error {
	if c == nil || rt == nil {
		return nil
	}

	// 用一份全新的 Router 承接本次编译产物，避免和旧路由表相互污染。
	router := runtime.NewRouter()

	for _, protocol := range c.protocolOrder() {
		cp := c.Protocols[protocol]
		if cp == nil || cp.Install == nil {
			continue
		}
		// 安装顺序保持稳定，避免多协议同时存在时因为 map 迭代顺序产生不确定行为。
		if err := cp.Install(router); err != nil {
			return err
		}
	}

	rt.SetRouter(router)
	if c.BindingRegistry != nil {
		rt.GetExecutor().WithBindingRegistry(c.BindingRegistry)
	}
	if c.RouteContainers != nil {
		rt.GetExecutor().SetRouteContainers(c.RouteContainers)
	}

	// 保留 runtime 级全局 AOP，兼容某些未完全走编译期 AOP 的执行路径。
	rt.UseGlobalMiddleware(c.GlobalAOP.Middlewares...)
	rt.UseGlobalGuards(c.GlobalAOP.Guards...)
	rt.UseGlobalInterceptors(c.GlobalAOP.Interceptors...)
	rt.UseGlobalPipes(c.GlobalAOP.Pipes...)
	rt.UseGlobalFilters(c.GlobalAOP.Filters...)

	return nil
}

// stableProtocolPlugins 对插件列表做过滤+稳定排序：
// - 过滤 nil
// - 按 protocol 字符串、再按 plugin ID 字符串排序
//
// 目的：保证编译结果稳定可复现（避免 map/切片迭代顺序导致的非确定性）。
func stableProtocolPlugins(plugins []ProtocolPlugin) []ProtocolPlugin {
	out := make([]ProtocolPlugin, 0, len(plugins))
	for _, p := range plugins {
		if p == nil {
			continue
		}
		out = append(out, p)
	}
	slices.SortStableFunc(out, func(a, b ProtocolPlugin) int {
		if a.Protocol() != b.Protocol() {
			return compareStrings(string(a.Protocol()), string(b.Protocol()))
		}
		return compareStrings(a.ID(), b.ID())
	})
	return out
}

// protocolOrder 返回编译产物的协议安装顺序。
//
// 规则：
// - 如果显式设置了 CompiledApp.ProtocolOrder，则返回其拷贝
// - 否则从 Protocols map 取 key 并排序（保证稳定）
func (c *CompiledApp) protocolOrder() []runtime.Protocol {
	if c == nil {
		return nil
	}
	if len(c.ProtocolOrder) > 0 {
		return append([]runtime.Protocol{}, c.ProtocolOrder...)
	}
	order := make([]runtime.Protocol, 0, len(c.Protocols))
	for protocol := range c.Protocols {
		order = append(order, protocol)
	}
	slices.Sort(order)
	return order
}

// compareStrings 是一个三态字符串比较函数，用于 slices.SortStableFunc。
func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
