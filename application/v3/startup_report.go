package application

import (
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// startup_report.go 实现 Application 的启动汇总报告模型与格式化输出。
//
// 这层是“编译诊断 + 应用装配信息”的对外展示面，
// 目标是让启动阶段的问题在一个报告里可读地串起来。

// StartupReport 汇总应用启动时的模块、适配器、协议与路由等统计信息。
//
// JSON 合同说明：
// - 该结构是 Application 侧稳定诊断输出面的一部分。
// - 对外 JSON 键名固定使用 lowerCamelCase。
// - compat fallback 相关字段与 CompileDiagnostics 保持同名同层级，便于外部工具复用。
type StartupReport struct {
	ModuleCount              int                                      `json:"moduleCount"`
	AdapterCount             int                                      `json:"adapterCount"`
	ProtocolCount            int                                      `json:"protocolCount"`
	RouteCount               int                                      `json:"routeCount"`
	BindingResolverCount     int                                      `json:"bindingResolverCount"`
	RegistrySource           string                                   `json:"registrySource"`
	CompatFallbackCategories []compiler.CompatFallbackCategorySummary `json:"compatFallbackCategories"`
	CompatFallbackSources    []compiler.CompatFallbackSourceSummary   `json:"compatFallbackSources"`
	GlobalAOP                compiler.AOPDiagnostics                  `json:"globalAOP"`
	RegistryFallbacks        []compiler.RegistryFallbackDiagnostics   `json:"registryFallbacks"`
	RuntimeFallbacks         []compiler.RuntimeFallbackDiagnostics    `json:"runtimeFallbacks"`
	Modules                  []ModuleStartupReport                    `json:"modules"`
	Adapters                 []AdapterStartupReport                   `json:"adapters"`
	Protocols                []ProtocolStartupReport                  `json:"protocols"`
	Integrations             []string                                 `json:"integrations"`
	Warnings                 []string                                 `json:"warnings"`
}

// HasCompatFallbacks 返回当前启动报告中是否存在 compat fallback 聚合结果。
func (r *StartupReport) HasCompatFallbacks() bool {
	return r != nil && (len(r.CompatFallbackCategories) > 0 || len(r.CompatFallbackSources) > 0)
}

// ModuleStartupReport 描述单个模块在启动汇总中的统计项。
type ModuleStartupReport struct {
	Name        string `json:"name"`
	Package     string `json:"package"`
	Imports     int    `json:"imports"`
	Providers   int    `json:"providers"`
	Controllers int    `json:"controllers"`
	Resolvers   int    `json:"resolvers"`
	Global      bool   `json:"global"`
}

// AdapterStartupReport 描述单个 adapter 的启动信息摘要。
type AdapterStartupReport struct {
	ID             string `json:"id"`
	SharedListener bool   `json:"sharedListener"`
}

// ProtocolStartupReport 描述单个协议在启动汇总中的编译结果摘要。
type ProtocolStartupReport struct {
	Protocol        runtime.Protocol                  `json:"protocol"`
	PluginID        string                            `json:"pluginId"`
	Declarations    int                               `json:"declarations"`
	Routes          int                               `json:"routes"`
	RouteContainers int                               `json:"routeContainers"`
	OwnerModules    []compiler.ModuleRouteDiagnostics `json:"ownerModules"`
}

// StartupReport 返回一次启动汇总报告（会在必要时触发编译）。
func (a *Application) StartupReport() (*StartupReport, error) {
	if a == nil {
		return nil, fmt.Errorf("application is nil")
	}
	if err := a.ensureCompiled(); err != nil {
		return nil, err
	}
	return a.buildStartupReport(), nil
}

// emitStartupReport 把启动报告输出到配置的 StartupReporter（若有）。
func (a *Application) emitStartupReport() error {
	if a == nil || a.startupReporter == nil {
		return nil
	}
	report, err := a.StartupReport()
	if err != nil {
		return err
	}
	if report == nil {
		return nil
	}
	_, err = fmt.Fprintln(a.startupReporter, report.String())
	return err
}

// buildStartupReport 组装最终报告：
// - 先收集模块与 adapter 视图
// - 再并入最近一次编译诊断的协议与 fallback 信息
func (a *Application) buildStartupReport() *StartupReport {
	report := &StartupReport{
		Adapters: make([]AdapterStartupReport, 0, len(a.adapterList)),
		Modules:  collectModuleReports(a.moduleRef),
	}
	report.ModuleCount = len(report.Modules)
	report.Integrations = collectIntegrations(report.Modules)

	for _, adp := range a.adapterList {
		if adp == nil {
			continue
		}
		item := AdapterStartupReport{ID: adp.ID()}
		if cfg, ok := adp.(adapter.SharedListenerConfigurator); ok {
			item.SharedListener = cfg.RequiresSharedListen()
		}
		report.Adapters = append(report.Adapters, item)
	}
	slices.SortFunc(report.Adapters, func(a, b AdapterStartupReport) int {
		return strings.Compare(a.ID, b.ID)
	})
	report.AdapterCount = len(report.Adapters)

	diag := a.LastCompileDiagnostics()
	if diag == nil {
		return report
	}
	report.BindingResolverCount = diag.BindingResolverCount
	report.RegistrySource = diag.RegistrySource
	report.CompatFallbackCategories = append([]compiler.CompatFallbackCategorySummary{}, diag.CompatFallbackCategories...)
	report.CompatFallbackSources = append([]compiler.CompatFallbackSourceSummary{}, diag.CompatFallbackSources...)
	report.GlobalAOP = diag.GlobalAOP
	report.RegistryFallbacks = append([]compiler.RegistryFallbackDiagnostics{}, diag.RegistryFallbacks...)
	report.RuntimeFallbacks = append([]compiler.RuntimeFallbackDiagnostics{}, diag.RuntimeFallbacks...)
	report.Warnings = append(report.Warnings, diag.Warnings...)
	report.Protocols = make([]ProtocolStartupReport, 0, len(diag.Protocols))
	for _, protocol := range diag.ProtocolOrder {
		pd := diag.Protocols[protocol]
		if pd == nil {
			continue
		}
		report.Protocols = append(report.Protocols, ProtocolStartupReport{
			Protocol:        protocol,
			PluginID:        pd.PluginID,
			Declarations:    pd.DeclarationCount,
			Routes:          pd.RouteCount,
			RouteContainers: pd.RouteContainers,
			OwnerModules:    append([]compiler.ModuleRouteDiagnostics{}, pd.OwnerModules...),
		})
		report.RouteCount += pd.RouteCount
	}
	if len(report.Protocols) == 0 {
		for protocol, pd := range diag.Protocols {
			if pd == nil {
				continue
			}
			report.Protocols = append(report.Protocols, ProtocolStartupReport{
				Protocol:        protocol,
				PluginID:        pd.PluginID,
				Declarations:    pd.DeclarationCount,
				Routes:          pd.RouteCount,
				RouteContainers: pd.RouteContainers,
				OwnerModules:    cloneOwnerModules(pd.OwnerModules),
			})
			report.RouteCount += pd.RouteCount
		}
		slices.SortFunc(report.Protocols, func(a, b ProtocolStartupReport) int {
			return strings.Compare(string(a.Protocol), string(b.Protocol))
		})
	}
	report.ProtocolCount = len(report.Protocols)
	return report
}

// String 以可读文本格式输出启动汇总信息。
func (r *StartupReport) String() string {
	if r == nil {
		return ""
	}
	lines := []string{
		"startup report:",
		fmt.Sprintf("  modules=%d adapters=%d protocols=%d routes=%d bindings=%d", r.ModuleCount, r.AdapterCount, r.ProtocolCount, r.RouteCount, r.BindingResolverCount),
		fmt.Sprintf("  registry=%s", strings.TrimSpace(defaultIfEmpty(r.RegistrySource, "unknown"))),
		fmt.Sprintf("  globalAOP middleware=%d guard=%d interceptor=%d pipe=%d filter=%d", r.GlobalAOP.Middlewares, r.GlobalAOP.Guards, r.GlobalAOP.Interceptors, r.GlobalAOP.Pipes, r.GlobalAOP.Filters),
	}
	if r.HasCompatFallbacks() {
		lines = append(lines, "  compatFallbacks:")
		if categories := formatCompatFallbackCategories(r.CompatFallbackCategories); categories != "" {
			lines = append(lines, fmt.Sprintf("    - fallbackCategories=%s", categories))
		}
		if sources := formatCompatFallbackSources(r.CompatFallbackSources); sources != "" {
			lines = append(lines, fmt.Sprintf("    - fallbackSources=%s", sources))
		}
	}
	if len(r.Integrations) > 0 {
		lines = append(lines, fmt.Sprintf("  integrations=%s", strings.Join(r.Integrations, ", ")))
	}
	if len(r.Adapters) > 0 {
		lines = append(lines, "  adapters:")
		for _, item := range r.Adapters {
			mode := "standalone"
			if item.SharedListener {
				mode = "shared-listener"
			}
			lines = append(lines, fmt.Sprintf("    - %s (%s)", item.ID, mode))
		}
	}
	if len(r.Protocols) > 0 {
		lines = append(lines, "  protocols:")
		for _, item := range r.Protocols {
			line := fmt.Sprintf("    - %s plugin=%s declarations=%d routes=%d containers=%d", item.Protocol, item.PluginID, item.Declarations, item.Routes, item.RouteContainers)
			if owners := formatOwnerModules(item.OwnerModules); owners != "" {
				line += fmt.Sprintf(" moduleOwners=%s", owners)
			}
			lines = append(lines, line)
			for _, owner := range item.OwnerModules {
				if details := formatRouteKeys(owner.RouteKeys, 4); details != "" {
					lines = append(lines, fmt.Sprintf("      moduleOwner=%s routes=%d keys=%s", owner.ModuleKey, owner.Routes, details))
				}
			}
		}
	}
	if len(r.RegistryFallbacks) > 0 {
		lines = append(lines, "  registryFallbacks:")
		for _, item := range r.RegistryFallbacks {
			lines = append(lines, fmt.Sprintf("    - %s category=%s summary=%s", item.Key, item.Category, item.Summary))
		}
	}
	if len(r.RuntimeFallbacks) > 0 {
		lines = append(lines, "  runtimeFallbacks:")
		for _, item := range r.RuntimeFallbacks {
			lines = append(lines, fmt.Sprintf("    - %s category=%s summary=%s", item.Key, item.Category, item.Summary))
		}
	}
	if len(r.Modules) > 0 {
		lines = append(lines, "  modules:")
		for _, item := range r.Modules {
			lines = append(lines, fmt.Sprintf("    - %s imports=%d providers=%d controllers=%d resolvers=%d global=%t", item.Name, item.Imports, item.Providers, item.Controllers, item.Resolvers, item.Global))
		}
	}
	if len(r.Warnings) > 0 {
		lines = append(lines, "  warnings:")
		for _, item := range r.Warnings {
			lines = append(lines, fmt.Sprintf("    - %s", item))
		}
	}
	return strings.Join(lines, "\n")
}

func defaultIfEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func formatOwnerModules(items []compiler.ModuleRouteDiagnostics) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if item.ModuleKey == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%d", item.ModuleKey, item.Routes))
	}
	return strings.Join(parts, ",")
}

func formatRouteKeys(items []string, limit int) string {
	if len(items) == 0 {
		return ""
	}
	if limit <= 0 || len(items) <= limit {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s ... (+%d more)", strings.Join(items[:limit], ", "), len(items)-limit)
}

func cloneOwnerModules(items []compiler.ModuleRouteDiagnostics) []compiler.ModuleRouteDiagnostics {
	if len(items) == 0 {
		return nil
	}
	out := make([]compiler.ModuleRouteDiagnostics, 0, len(items))
	for _, item := range items {
		copyItem := item
		copyItem.RouteKeys = append([]string{}, item.RouteKeys...)
		out = append(out, copyItem)
	}
	return out
}

// collectModuleReports 从 ModuleRef 快照化模块统计信息。
func collectModuleReports(moduleRef *module.ModuleRef) []ModuleStartupReport {
	if moduleRef == nil {
		return nil
	}
	reports := make([]ModuleStartupReport, 0)
	for moduleType, mod := range moduleRef.GetAllModulesWithTypes() {
		if mod == nil || mod.Metadata() == nil {
			continue
		}
		meta := mod.Metadata()
		pkgPath := moduleType.PkgPath()
		if moduleType.Kind() == reflect.Ptr {
			pkgPath = moduleType.Elem().PkgPath()
		}
		reports = append(reports, ModuleStartupReport{
			Name:        moduleType.String(),
			Package:     pkgPath,
			Imports:     len(meta.Imports),
			Providers:   len(meta.Providers),
			Controllers: len(meta.Controllers),
			Resolvers:   len(meta.Resolvers),
			Global:      meta.IsGlobal,
		})
	}
	slices.SortFunc(reports, func(a, b ModuleStartupReport) int {
		return strings.Compare(a.Name, b.Name)
	})
	return reports
}

// collectIntegrations 从模块包路径里提取 integrations 名称，用于启动摘要展示。
func collectIntegrations(modules []ModuleStartupReport) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, item := range modules {
		if item.Package == "" {
			continue
		}
		if !strings.Contains(item.Package, "/integrations/") {
			continue
		}
		name := path.Base(item.Package)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}
