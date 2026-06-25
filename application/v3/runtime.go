// runtime.go 实现 Application 的编译、启动和共享监听协调逻辑。
//
// 这一层回答的是：
// - 何时触发编译
// - 编译失败时诊断如何保留
// - Run / Listen 这两种启动方式如何分工
package application

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// Start 是 Run 的别名，用来保留更熟悉的应用生命周期调用方式。
func (a *Application) Start() error { return a.Run() }

// Run 会在需要时完成编译，触发 bootstrap 生命周期钩子，
// 然后按各 adapter 自己的监听配置启动它们。
//
// 当各 adapter 已经各自完成配置时，直接用 Run 即可。
// 如果某个 adapter 需要共享监听地址，应改用 Listen。
func (a *Application) Run() error {
	if err := a.ensureCompiled(); err != nil {
		return err
	}
	if err := a.bootstrapLifecycle(); err != nil {
		return err
	}
	for _, adp := range a.adapterList {
		if cfg, ok := adp.(adapter.SharedListenerConfigurator); ok && cfg.RequiresSharedListen() {
			return fmt.Errorf("shared listener adapters present; use app.Listen(addr)")
		}
	}
	if err := a.emitStartupReport(); err != nil {
		return err
	}
	return a.startAdapters()
}

// Stop 按 adapter 挂载的逆序停止服务，然后执行 shutdown 生命周期钩子。
func (a *Application) Stop() error {
	for i := len(a.adapterList) - 1; i >= 0; i-- {
		if err := a.adapterList[i].Stop(); err != nil {
			return err
		}
	}
	return a.shutdownLifecycle()
}

// ensureCompiled 保证当前应用已经完成一次成功编译。
// 编译结果一旦缓存，后续重复调用不会重复构建 runtime。
func (a *Application) ensureCompiled() error {
	if a.compiled != nil {
		return nil
	}
	rt, compiled, diag, err := a.buildCompiledRuntime()
	if err != nil {
		return err
	}
	a.runtime = rt
	a.compiled = compiled
	a.lastDiagnostics = diag
	return nil
}

// buildCompiledRuntime 从 `moduleRef + registry + plugins` 重新构建一套 runtime。
// HotLoad 和首次启动都会复用这条路径。
func (a *Application) buildCompiledRuntime() (*runtime.Runtime, *compiler.CompiledApp, *compiler.CompileDiagnostics, error) {
	rt := runtime.NewRuntime()
	if a.moduleRef != nil && a.moduleRef.GetContainer() != nil {
		rt.GetExecutor().SetRootContainer(a.moduleRef.GetContainer())
	}
	compiled, err := compiler.Compile(a.moduleRef, a.registry, a.plugins...)
	if err != nil {
		var compileErr *compiler.CompileError
		if errors.As(err, &compileErr) && compileErr.Diagnostics != nil {
			// 编译失败时仍尽量保留已知诊断，方便启动前排障。
			a.lastDiagnostics = a.decorateCompileDiagnostics(compileErr.Diagnostics)
		}
		return nil, nil, a.LastCompileDiagnostics(), err
	}
	if compiled != nil && compiled.Diagnostics != nil {
		compiled.Diagnostics = a.decorateCompileDiagnostics(compiled.Diagnostics)
	}
	if err := compiled.Install(rt); err != nil {
		return nil, nil, nil, err
	}
	return rt, compiled, compiled.Diagnostics.Clone(), nil
}

// registryWarnings 基于当前 registrySource 和 fallbackUsage 生成启动警告。
// 它主要帮助从 compat/global registry 逐步迁移到实例级 registry。
func (a *Application) registryWarnings() []string {
	if a == nil {
		return nil
	}
	warnings := make([]string, 0, 2)
	if a.registrySource == "global-default" {
		msg := "application is using registry.Global() via default fallback; prefer application.NewWithOptions(..., Options{Registry: application.NewRegistry()}) or use application.NewCompat(...) to make compatibility explicit"
		if a.explicitCompat {
			msg = "application is using registry.Global() via explicit compatibility path application.NewCompat(...); prefer application.NewWithOptions(..., Options{Registry: application.NewRegistry()}) for new code"
		}
		if summary := summarizeRegistryUsage(a.registry); summary != "" {
			msg += "; current registry contains " + summary
		}
		warnings = append(warnings, msg)
		if eventsCompat := summarizeEventsCompatUsage(a.registry.SnapshotFallbackUsage()); eventsCompat != "" {
			warnings = append(warnings,
				fmt.Sprintf("migration-only events compat writes are active on global registry (%s); for new code prefer events.RegisterOn/RegisterOnce/RegisterHandlers with an explicit registry instance", eventsCompat),
			)
		}
		return warnings
	}
	if a.registry == nil || a.registry == registry.Global() {
		return warnings
	}
	globalUsage := registry.Global().SnapshotFallbackUsage()
	if summary := summarizeRegistryUsage(registry.Global()); summary != "" {
		warnings = append(warnings,
			fmt.Sprintf("global registry also contains %s; these writes likely came from migration-only global entrypoints such as package-level DSL or *Compat APIs and are ignored by the current application instance", summary),
		)
	}
	if eventsCompat := summarizeEventsCompatUsage(globalUsage); eventsCompat != "" {
		warnings = append(warnings,
			fmt.Sprintf("ignored migration-only global events compat writes detected (%s); prefer events.RegisterOn/RegisterOnce/RegisterHandlers with the application registry instance", eventsCompat),
		)
	}
	return warnings
}

// decorateCompileDiagnostics 在 compiler 产出的原始诊断上再附加 application 侧上下文。
func (a *Application) decorateCompileDiagnostics(diag *compiler.CompileDiagnostics) *compiler.CompileDiagnostics {
	if diag == nil {
		return nil
	}
	out := diag.Clone()
	out.RegistrySource = a.registrySource
	out.CompatFallbackCategories, out.CompatFallbackSources = summarizeCompatFallbackUsage(a)
	out.Warnings = append(out.Warnings, a.registryWarnings()...)
	return out
}

func summarizeRegistryUsage(reg *registry.Registry) string {
	if reg == nil {
		return ""
	}
	parts := make([]string, 0, 5)
	if summary := summarizeDeclUsage(reg.SnapshotDeclCounts()); summary != "" {
		parts = append(parts, summary)
	}
	if total := len(reg.GetBindings()); total > 0 {
		parts = append(parts, fmt.Sprintf("%d bindings", total))
	}
	if total := totalGlobalAOPCount(reg.GetGlobalAOP()); total > 0 {
		parts = append(parts, fmt.Sprintf("%d global AOP entries", total))
	}
	fallbackUsage := reg.SnapshotFallbackUsage()
	categoryItems, sourceItems := collectCompatFallbackSummaries(fallbackUsage)
	if categories := formatCompatFallbackCategories(categoryItems); categories != "" {
		parts = append(parts, "fallbackCategories="+categories)
	}
	if sources := formatCompatFallbackSources(sourceItems); sources != "" {
		parts = append(parts, "fallbackSources="+sources)
	}
	return joinSummaryParts(parts)
}

func joinSummaryParts(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	default:
		head := strings.Join(parts[:len(parts)-1], ", ")
		return head + ", and " + parts[len(parts)-1]
	}
}

func summarizeDeclUsage(items map[string]map[string]int) string {
	total := totalDeclCount(items)
	if total == 0 {
		return ""
	}
	breakdown := make([]string, 0)
	for pluginID, kinds := range items {
		for kind, count := range kinds {
			if count <= 0 {
				continue
			}
			breakdown = append(breakdown, fmt.Sprintf("%s.%s=%d", pluginID, kind, count))
		}
	}
	slices.Sort(breakdown)
	if len(breakdown) == 0 {
		return fmt.Sprintf("%d declarations", total)
	}
	if len(breakdown) > 4 {
		extra := len(breakdown) - 4
		breakdown = append(append([]string{}, breakdown[:4]...), fmt.Sprintf("+%d more", extra))
	}
	return fmt.Sprintf("%d declarations (%s)", total, strings.Join(breakdown, ", "))
}

func collectCompatFallbackSummaries(items map[string]int) ([]compiler.CompatFallbackCategorySummary, []compiler.CompatFallbackSourceSummary) {
	if len(items) == 0 {
		return nil, nil
	}
	type categorySummary struct {
		hits    int
		sources int
	}
	categories := make(map[string]*categorySummary)
	sourceItems := make([]compiler.CompatFallbackSourceSummary, 0, len(items))
	for source, count := range items {
		if count <= 0 {
			continue
		}
		sourceItems = append(sourceItems, compiler.CompatFallbackSourceSummary{
			Source: source,
			Hits:   count,
		})
		key := classifyFallbackSource(source)
		entry := categories[key]
		if entry == nil {
			entry = &categorySummary{}
			categories[key] = entry
		}
		entry.hits += count
		entry.sources++
	}
	if len(categories) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(categories))
	for key := range categories {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	categoryItems := make([]compiler.CompatFallbackCategorySummary, 0, len(keys))
	for _, key := range keys {
		item := categories[key]
		categoryItems = append(categoryItems, compiler.CompatFallbackCategorySummary{
			Category: key,
			Hits:     item.hits,
			Sources:  item.sources,
		})
	}
	slices.SortFunc(sourceItems, func(a, b compiler.CompatFallbackSourceSummary) int {
		return strings.Compare(a.Source, b.Source)
	})
	return categoryItems, sourceItems
}

func formatCompatFallbackSources(items []compiler.CompatFallbackSourceSummary) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if item.Hits <= 0 || strings.TrimSpace(item.Source) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%d", item.Source, item.Hits))
	}
	if len(parts) == 0 {
		return ""
	}
	if len(parts) > 4 {
		extra := len(parts) - 4
		parts = append(append([]string{}, parts[:4]...), fmt.Sprintf("+%d more", extra))
	}
	return strings.Join(parts, ", ")
}

func formatCompatFallbackCategories(items []compiler.CompatFallbackCategorySummary) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if item.Hits <= 0 || strings.TrimSpace(item.Category) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%d hit/%d source", item.Category, item.Hits, item.Sources))
	}
	return strings.Join(parts, ", ")
}

func summarizeCompatFallbackUsage(a *Application) ([]compiler.CompatFallbackCategorySummary, []compiler.CompatFallbackSourceSummary) {
	if a == nil {
		return nil, nil
	}
	var usage map[string]int
	switch {
	case a.registrySource == "global-default":
		if a.registry != nil {
			usage = a.registry.SnapshotFallbackUsage()
		}
	case a.registry != nil && a.registry != registry.Global():
		usage = registry.Global().SnapshotFallbackUsage()
	default:
		if a.registry != nil {
			usage = a.registry.SnapshotFallbackUsage()
		}
	}
	return collectCompatFallbackSummaries(usage)
}

func classifyFallbackSource(source string) string {
	switch {
	case strings.HasSuffix(source, ".NewCompatAPI()"):
		return "adapterCompatAPI"
	case strings.HasPrefix(source, "binding/"):
		return "bindingCompat"
	case strings.HasPrefix(source, "core/integration."):
		return "containerBindingCompat"
	case strings.HasPrefix(source, "integrations/events.Register"):
		return "eventsCompatWrites"
	case strings.HasPrefix(source, "integrations/"):
		return "integrationCompat"
	default:
		return "packageDSL"
	}
}

func summarizeEventsCompatUsage(items map[string]int) string {
	if len(items) == 0 {
		return ""
	}
	keys := []string{
		"integrations/events.RegisterOnCompat",
		"integrations/events.RegisterOnceCompat",
		"integrations/events.RegisterHandlersCompat",
	}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if count := items[key]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", key, count))
		}
	}
	return strings.Join(parts, ", ")
}

func totalDeclCount(items map[string]map[string]int) int {
	total := 0
	for _, kinds := range items {
		for _, count := range kinds {
			total += count
		}
	}
	return total
}

func totalGlobalAOPCount(items registry.GlobalAOPRegistration) int {
	return len(items.Middlewares) +
		len(items.Guards) +
		len(items.Interceptors) +
		len(items.Pipes) +
		len(items.Filters)
}

// Listen 使用 addr 作为共享监听地址来启动应用。
//
// 独立监听模式的 adapter 仍然使用自己的地址配置；
// 需要共享监听的 adapter 会在启动前通过 ConfigureSharedListen 收到这个 addr。
func (a *Application) Listen(addr string) error {
	if err := a.ensureCompiled(); err != nil {
		return err
	}
	if err := a.bootstrapLifecycle(); err != nil {
		return err
	}
	sharedAdapters := make([]adapter.SharedListenerConfigurator, 0, 1)
	for _, adp := range a.adapterList {
		if cfg, ok := adp.(adapter.SharedListenerConfigurator); ok && cfg.RequiresSharedListen() {
			sharedAdapters = append(sharedAdapters, cfg)
		}
	}
	if len(sharedAdapters) > 1 {
		return fmt.Errorf("multiple adapters require shared listener address")
	}
	if len(sharedAdapters) == 1 {
		if err := sharedAdapters[0].ConfigureSharedListen(addr); err != nil {
			return err
		}
	}
	if err := a.emitStartupReport(); err != nil {
		return err
	}
	return a.startAdapters()
}

// startAdapters 按挂载顺序启动所有 adapter。
// 共享监听地址的协商应在进入这里之前已经完成。
func (a *Application) startAdapters() error {
	for _, adp := range a.adapterList {
		if err := adp.Start(); err != nil {
			return err
		}
	}
	return nil
}
