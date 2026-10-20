// application.go 组装并驱动一个 lania-g 应用。
//
// 唯一的装配入口是 New：它从根模块出发完成
//
//	加载模块树 → Init → 发现适配器 → 挂载适配器 → 声明协议路由 → 合并全局 AOP
//
// 唯一的运行入口是 Run：它完成编译、启动适配器并阻塞到 Stop。
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
	"sync"
)

// Application 持有模块树、声明仓库、适配器与运行期状态。
type Application struct {
	runtime         *runtime.Runtime
	registry        *registry.Registry
	moduleLoader    *module.ModuleLoader
	moduleRef       *module.ModuleRef
	adapters        map[string]adapter.Adapter
	adapterList     []adapter.Adapter
	compiled        *compiler.CompiledApp
	lastDiagnostics *compiler.CompileDiagnostics
	plugins         []compiler.ProtocolPlugin
	startupReporter io.Writer
	bootstrapped    bool
	prepared        bool
	mounted         bool

	stopCh   chan struct{}
	stopOnce sync.Once
}

// Options 描述创建 Application 时的基础选项。
type Options struct {
	// Logger 非空时输出启动报告。
	Logger io.Writer
}

// New 基于根模块创建应用。
//
// root 是唯一的装配入口：协议适配器、集成模块、控制器都必须挂在它下面。
func New(root module.Module, opts Options) (*Application, error) {
	if root == nil {
		return nil, fmt.Errorf("application.New requires a root module")
	}

	reg := registry.New()
	loader := module.NewModuleLoader()
	loader.SetRegistry(reg)
	if _, err := loader.LoadMultiple(root); err != nil {
		return nil, err
	}

	rt := runtime.NewRuntime()
	if ref := loader.GetModuleRef(); ref != nil {
		rt.GetExecutor().SetRootContainer(ref.GetContainer())
	}

	app := &Application{
		runtime:         rt,
		registry:        reg,
		moduleLoader:    loader,
		moduleRef:       loader.GetModuleRef(),
		adapters:        make(map[string]adapter.Adapter),
		adapterList:     make([]adapter.Adapter, 0),
		startupReporter: opts.Logger,
		stopCh:          make(chan struct{}),
	}

	// 模块在 Init 阶段贡献的全局 AOP 与 registry 上的全局 AOP 合并成一份。
	contribution := loader.GetAOP()
	reg.UseGlobalMiddlewares(contribution.Middlewares...)
	reg.UseGlobalGuards(contribution.Guards...)
	reg.UseGlobalInterceptors(contribution.Interceptors...)
	reg.UseGlobalPipes(contribution.Pipes...)
	reg.UseGlobalFilters(contribution.Filters...)
	// 范围化中间件来自模块的 Configure(consumer)，编译期按 route method/path 展开。
	reg.SetScopedMiddlewares(loader.GetScopedMiddlewares()...)

	if err := app.mountModuleAdapters(root); err != nil {
		return nil, err
	}
	return app, nil
}

// mountModuleAdapters 从模块树中发现并挂载适配器。
func (a *Application) mountModuleAdapters(root module.Module) error {
	return module.WalkModules(root, func(mod module.Module) error {
		provider, ok := mod.(adapter.ModuleAdapter)
		if !ok {
			return nil
		}
		return a.useAdapter(provider.Adapter())
	})
}

// Runtime 返回当前应用使用的 runtime 实例。
func (a *Application) Runtime() *runtime.Runtime { return a.runtime }

// Registry 返回当前应用的声明仓库。
//
// 它主要供集成与协议内部使用；业务代码不应依赖。
func (a *Application) Registry() *registry.Registry { return a.registry }

// ModuleRef 返回根 ModuleRef 树。
func (a *Application) ModuleRef() *module.ModuleRef { return a.moduleRef }

// GraphDiagnostics 返回模块加载图的诊断信息。
func (a *Application) GraphDiagnostics() *graph.Diagnostics {
	if a.moduleLoader == nil {
		return nil
	}
	return a.moduleLoader.GetDiagnostics()
}

// LastCompileDiagnostics 返回最近一次编译诊断快照。
func (a *Application) LastCompileDiagnostics() *compiler.CompileDiagnostics {
	if a == nil || a.lastDiagnostics == nil {
		return nil
	}
	return a.lastDiagnostics.Clone()
}

// Diagnostics 返回最近一次编译诊断快照。
func (a *Application) Diagnostics() *compiler.CompileDiagnostics {
	return a.LastCompileDiagnostics()
}

// RegisterPlugin 追加额外的协议插件。
func (a *Application) RegisterPlugin(plugins ...compiler.ProtocolPlugin) {
	for _, p := range plugins {
		if p != nil {
			a.plugins = append(a.plugins, p)
		}
	}
}

// useAdapter 挂载一个 adapter 并登记它暴露的协议插件。
func (a *Application) useAdapter(adp adapter.Adapter) error {
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
		a.RegisterPlugin(pp.Plugins()...)
	}
	return nil
}

// Adapter 返回某个已挂载的适配器。
func (a *Application) Adapter(adapterID string) (adapter.Adapter, bool) {
	adp, ok := a.adapters[adapterID]
	return adp, ok
}

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
