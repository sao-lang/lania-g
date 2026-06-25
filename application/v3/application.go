// application.go 定义 v3 应用入口，以及“模块树 + runtime + adapters + registry”的总装配逻辑。
//
// 如果说 module/runtime/compiler 是框架内部骨架，
// 那 Application 就是把这些骨架真正组装成一个可启动实例的外层门面。
package application

import (
	"fmt"
	"io"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/graph"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// Application 是 v3 的应用入口，负责把以下部分装配起来：
// - module loader（模块树与 DI 容器）
// - runtime（router/binding/pipeline/executor）
// - adapters（协议适配器与协议插件）
// - registry（各 adapter 写入的声明）
//
// 它本身不承载 Controller/Gateway 这类协议 DSL，
// 这些声明入口应放在对应的 adapter 中。
type Application struct {
	runtime         *runtime.Runtime
	registry        *registry.Registry
	registrySource  string
	explicitCompat  bool
	moduleLoader    *module.ModuleLoader
	moduleRef       *module.ModuleRef
	adapters        map[string]adapter.Adapter
	adapterList     []adapter.Adapter
	compiled        *compiler.CompiledApp
	lastDiagnostics *compiler.CompileDiagnostics
	plugins         []compiler.ProtocolPlugin
	startupReporter io.Writer
	bootstrapped    bool
}

// Options 描述创建 Application 时的基础选项。
type Options struct {
	Registry        *Registry
	StartupReporter io.Writer
}

// New 基于根模块创建一个 Application，并挂载传入的 adapters。
//
// 这是一个便捷入口；新业务代码更推荐使用 `NewWithOptions(...)` 并显式注入
// 实例级 `application.NewRegistry()`。返回的应用已经完成模块加载与 adapter 挂载，
// 但还没有开始监听端口或启动对外服务。
func New(root module.Module, adapters ...adapter.Adapter) (*Application, error) {
	return newWithOptions(root, Options{Registry: registry.Global()}, false, adapters...)
}

// NewCompat 基于根模块创建一个 Application，并显式保留“默认回退到全局 registry”的兼容语义。
func NewCompat(root module.Module, adapters ...adapter.Adapter) (*Application, error) {
	return newWithOptions(root, Options{Registry: registry.Global()}, true, adapters...)
}

// NewWithOptions 与 New 类似，但允许显式传入基础设施级选项。
//
// 常见用途：
// - 显式注入 Registry，而不是使用 registry.Global()
// - 打开 StartupReporter，观察编译结果与启动汇总
//
// 推荐：通过 `Options{Registry: NewRegistry()}` 显式注入实例级 registry。
// 兼容：若明确需要保留默认全局 registry 语义，优先使用 `NewCompat(...)`。
func NewWithOptions(root module.Module, opts Options, adapters ...adapter.Adapter) (*Application, error) {
	return newWithOptions(root, opts, false, adapters...)
}

func newWithOptions(root module.Module, opts Options, explicitCompat bool, adapters ...adapter.Adapter) (*Application, error) {
	reg := opts.Registry
	registrySource := "instance"
	if reg == nil {
		return nil, fmt.Errorf("application.NewWithOptions requires an explicit registry; use application.NewCompat(...) to keep registry.Global() compatibility")
	}
	if reg == registry.Global() {
		registrySource = "global-default"
	}
	loader := module.NewModuleLoader()
	loader.SetRegistry(reg)
	if _, err := loader.LoadMultiple(root); err != nil {
		return nil, err
	}

	rt := runtime.NewRuntime()
	if ref := loader.GetModuleRef(); ref != nil {
		// 把 root container 提前接给 runtime.Executor，
		// 这样后续请求执行时 child container 就都从同一棵模块容器树派生。
		rt.GetExecutor().SetRootContainer(ref.GetContainer())
	}

	app := &Application{
		runtime:         rt,
		registry:        reg,
		registrySource:  registrySource,
		explicitCompat:  explicitCompat,
		moduleLoader:    loader,
		moduleRef:       loader.GetModuleRef(),
		adapters:        make(map[string]adapter.Adapter),
		adapterList:     make([]adapter.Adapter, 0),
		startupReporter: opts.StartupReporter,
	}

	for _, adp := range adapters {
		if err := app.UseAdapter(adp); err != nil {
			return nil, err
		}
	}
	return app, nil
}

// Runtime 返回当前应用使用的 runtime 实例。
func (a *Application) Runtime() *runtime.Runtime { return a.runtime }

// Registry 返回当前应用使用的声明注册表。
func (a *Application) Registry() *Registry { return a.registry }

// ModuleRef 返回已加载完成的根 ModuleRef 树。
func (a *Application) ModuleRef() *module.ModuleRef { return a.moduleRef }

// GraphDiagnostics 返回模块加载图的诊断信息。
//
// 当你排查循环依赖、provider 缺失或模块归属异常时，这个信息会比较有用。
func (a *Application) GraphDiagnostics() *graph.Diagnostics {
	if a.moduleLoader == nil {
		return nil
	}
	return a.moduleLoader.GetDiagnostics()
}

// LastCompileDiagnostics 返回最近一次保存的编译诊断快照。
//
// 如果应用尚未触发编译，或者当前还没有可用诊断信息，则返回 nil。
func (a *Application) LastCompileDiagnostics() *compiler.CompileDiagnostics {
	if a == nil || a.lastDiagnostics == nil {
		return nil
	}
	return a.lastDiagnostics.Clone()
}

// CompileDiagnostics 在需要时触发编译，并返回一份诊断快照。
//
// 这很适合作为启动前预检查：在 Run 或 Listen 前先调用一次，
// 让路由冲突、binding 问题、声明错误尽早暴露出来。
func (a *Application) CompileDiagnostics() (*compiler.CompileDiagnostics, error) {
	if a == nil {
		return nil, fmt.Errorf("application is nil")
	}
	if err := a.ensureCompiled(); err != nil {
		return a.LastCompileDiagnostics(), err
	}
	return a.LastCompileDiagnostics(), nil
}

// RegisterPlugin 向应用追加额外的协议插件。
//
// 大多数业务代码不需要直接调用它，因为 adapter 通常会在 UseAdapter 时
// 自动暴露自己的协议插件。这个方法主要面向框架扩展场景。
func (a *Application) RegisterPlugin(plugins ...compiler.ProtocolPlugin) {
	for _, p := range plugins {
		if p != nil {
			// plugin 列表允许 adapter 和外部扩展同时追加；
			// 真正编译时会再做稳定排序和去 nil。
			a.plugins = append(a.plugins, p)
		}
	}
}

// UseAdapter 将一个 adapter 挂载到应用上。
//
// 挂载过程中，adapter 可能会写入声明、暴露 adapter 专属 API，
// 并为后续编译阶段提供协议插件。但它不会立刻启动；
// 真正启动仍发生在 Run 或 Listen 中。
func (a *Application) UseAdapter(adp adapter.Adapter) error {
	if adp == nil {
		return fmt.Errorf("adapter is nil")
	}
	id := adp.ID()
	if id == "" {
		return fmt.Errorf("adapter id is empty")
	}
	if _, exists := a.adapters[id]; exists {
		return fmt.Errorf("adapter already mounted: %s", id)
	}
	if err := adp.Mount(a); err != nil {
		return err
	}
	a.adapters[id] = adp
	a.adapterList = append(a.adapterList, adp)
	if pp, ok := adp.(interface {
		Plugins() []compiler.ProtocolPlugin
	}); ok {
		// 让 adapter 自己声明“它暴露哪些协议插件”，
		// Application 不需要了解每种 adapter 的内部细节。
		a.RegisterPlugin(pp.Plugins()...)
	}
	return nil
}

// API 返回某个已挂载 adapter 暴露出来的专属 API 对象。
//
// 当调用方需要在挂载后使用某些 adapter 专属能力时，可以通过这里获取，
// 例如 HTTP adapter 提供的辅助方法。
func (a *Application) API(adapterID string) (any, bool) {
	adp, ok := a.adapters[adapterID]
	if !ok {
		return nil, false
	}
	return adp.API(), true
}

// ===== v2 兼容糖封装（仍通过接口断言保留插件架构） =====

// UseGlobalMiddlewares 注册全局 middlewares（对所有协议路由生效）。
func (a *Application) UseGlobalMiddlewares(mws ...aop.MiddlewareFunc) {
	a.registry.UseGlobalMiddlewares(mws...)
}

// UseGlobalGuards 注册全局 guards（对所有协议路由生效）。
func (a *Application) UseGlobalGuards(gs ...aop.GuardFunc) {
	a.registry.UseGlobalGuards(gs...)
}

// UseGlobalInterceptors 注册全局 interceptors（对所有协议路由生效）。
func (a *Application) UseGlobalInterceptors(is ...aop.InterceptorFunc) {
	a.registry.UseGlobalInterceptors(is...)
}

// UseGlobalPipes 注册全局 pipes（对所有协议路由生效）。
func (a *Application) UseGlobalPipes(ps ...aop.PipeFunc) {
	a.registry.UseGlobalPipes(ps...)
}

// UseGlobalFilters 注册全局 exception filters（对所有协议路由生效）。
func (a *Application) UseGlobalFilters(fs ...aop.ExceptionFilterFunc) {
	a.registry.UseGlobalFilters(fs...)
}

// SetGlobalPrefix 为已挂载的 HTTP adapter 配置全局路由前缀。
func (a *Application) SetGlobalPrefix(prefix string) {
	adp, ok := a.adapters["http"]
	if !ok {
		return
	}
	if setter, ok := adp.(interface{ SetGlobalPrefix(string) }); ok {
		setter.SetGlobalPrefix(prefix)
	}
}
