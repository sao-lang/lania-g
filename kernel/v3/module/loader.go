// loader.go 实现模块树的静态加载、全局模块注入与依赖图校验。
//
// 如果说 `moduleloader.go` 处理的是运行期动态增减模块，
// 那这一层更像应用启动期的“首次建图”入口。
package module

import (
	"reflect"
	"sync"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/graph"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// ModuleLoader 负责从 root module 出发递归收集：
// - modules/imports 关系
// - providers/controllers/resolvers 声明
// 并生成依赖图诊断信息（循环依赖等）。
//
// 注意：ModuleLoader 本身不负责把 handler 注册到 runtime.Router，
// 这由 compiler + adapters 完成；ModuleLoader 主要管理 DI 侧的模块树。
type ModuleLoader struct {
	modules       map[Module]bool
	providers     map[interface{}]*di.Provider
	controllers   []interface{}
	rootModule    Module
	loadedModules map[reflect.Type]Module
	moduleRef     *ModuleRef
	diagnostics   *graph.Diagnostics
	registry      *registry.Registry
	mu            sync.RWMutex
}

// NewModuleLoader 创建模块加载器，并可选设置 root modules。
//
// 当传入多个 root 时，会在内部创建一个聚合 root（见 setRootModules）。
func NewModuleLoader(rootModules ...Module) *ModuleLoader {
	loader := &ModuleLoader{
		modules:       make(map[Module]bool),
		providers:     make(map[interface{}]*di.Provider),
		controllers:   make([]interface{}, 0),
		loadedModules: make(map[reflect.Type]Module),
	}
	loader.setRootModules(rootModules)
	return loader
}

// Load 加载单个 root module（LoadMultiple 的简写）。
func (l *ModuleLoader) Load(root Module) (*LoadResult, error) {
	return l.LoadMultiple(root)
}

// LoadMultiple 加载多个 root module。
//
// 约定：
// - 当 roots > 1 时，会创建一个“聚合 root”来承载 imports（见 setRootModules）
// - prepareGlobalModules 会把标记为 IsGlobal 的模块注入到所有模块的 imports 里（类似全局模块）
func (l *ModuleLoader) LoadMultiple(roots ...Module) (*LoadResult, error) {
	l.modules = make(map[Module]bool)
	l.providers = make(map[interface{}]*di.Provider)
	l.controllers = make([]interface{}, 0)
	l.setRootModules(roots)

	for _, root := range roots {
		if err := l.loadModule(root); err != nil {
			return nil, err
		}
	}

	// 先把所有声明收集完整，再做 global 注入与依赖图校验，
	// 最后才真正执行 root.Init() 触发 provider 实例化和生命周期。
	l.prepareGlobalModules()
	if err := l.validateGraphs(); err != nil {
		return nil, err
	}

	if l.rootModule != nil {
		if err := l.rootModule.Init(); err != nil {
			return nil, err
		}
	}

	return &LoadResult{
		Modules:     l.getModulesList(),
		Providers:   l.providers,
		Controllers: l.controllers,
	}, nil
}

// setRootModules 设置 rootModule/moduleRef，并重建 loadedModules 索引。
//
// 规则：
// - 0 个 root：清空 root 与 moduleRef
// - 1 个 root：直接使用该 module
// - >1 个 root：创建聚合 root（CreateModule）
func (l *ModuleLoader) setRootModules(roots []Module) {
	l.mu.Lock()
	l.loadedModules = make(map[reflect.Type]Module)

	if len(roots) == 0 {
		l.rootModule = nil
		l.moduleRef = nil
		l.mu.Unlock()
		return
	}

	if len(roots) == 1 {
		l.rootModule = roots[0]
	} else {
		l.rootModule = CreateModule(roots, nil, nil, nil, nil)
	}

	l.moduleRef = NewModuleRef(l.rootModule)
	l.mu.Unlock()
	l.collectLoadedModules(l.rootModule)
}

// GetModuleRef 返回当前 loader 持有的 ModuleRef。
func (l *ModuleLoader) GetModuleRef() *ModuleRef {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.moduleRef
}

// GetDiagnostics 返回最近一次 validateGraphs 生成的诊断信息。
func (l *ModuleLoader) GetDiagnostics() *graph.Diagnostics {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.diagnostics
}

// GetRootModule 返回当前 root module。
func (l *ModuleLoader) GetRootModule() Module {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.rootModule
}

// SetRegistry 设置 Registry，并向当前 root 树传播 RegistryAware 能力。
func (l *ModuleLoader) SetRegistry(reg *registry.Registry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.registry = reg
	if l.rootModule != nil {
		l.propagateRegistry(l.rootModule)
	}
}

// loadModule 递归加载一个模块及其 imports，并收集 providers/controllers。
//
// 这是“声明收集”阶段，不负责 runtime handler 注册。
func (l *ModuleLoader) loadModule(mod Module) error {
	l.propagateRegistry(mod)
	if l.modules[mod] {
		return nil
	}

	l.modules[mod] = true

	metadata := mod.Metadata()
	for _, imp := range metadata.Imports {
		if err := l.loadModule(imp); err != nil {
			return err
		}
	}

	for _, provider := range metadata.Providers {
		l.providers[provider.Token] = provider
	}

	l.controllers = append(l.controllers, metadata.Controllers...)

	return nil
}

// propagateRegistry 将 registry 注入实现了 RegistryAware 的模块。
// 这让模块内部的 DSL/API 可以感知当前应用 registry，而不必层层手传。
func (l *ModuleLoader) propagateRegistry(mod Module) {
	if mod == nil || l.registry == nil {
		return
	}
	if aware, ok := mod.(RegistryAware); ok {
		aware.SetRegistry(l.registry)
	}
}

// getModulesList 返回当前已加载模块集合的切片快照。
func (l *ModuleLoader) getModulesList() []Module {
	list := make([]Module, 0, len(l.modules))
	for mod := range l.modules {
		list = append(list, mod)
	}
	return list
}

// prepareGlobalModules 把声明为 IsGlobal 的模块注入到所有模块的 imports 中。
//
// 这样业务模块即使没有显式 import，也能共享这些全局模块导出的能力。
func (l *ModuleLoader) prepareGlobalModules() {
	modules := l.getModulesList()
	globals := make([]Module, 0)
	for _, mod := range modules {
		if mod != nil && mod.Metadata() != nil && mod.Metadata().IsGlobal {
			globals = append(globals, mod)
		}
	}
	if len(globals) == 0 {
		return
	}
	// 将 global module 注入到每个 module 的 imports 中，避免用户显式 import。
	for _, mod := range modules {
		if mod == nil || mod.Metadata() == nil {
			continue
		}
		for _, global := range globals {
			if mod == global || hasImportedModule(mod.Metadata().Imports, global) {
				continue
			}
			mod.Metadata().Imports = append(mod.Metadata().Imports, global)
		}
	}
}

// validateGraphs 构建模块/Provider 依赖图并生成诊断信息。
// 当存在循环依赖等问题时，会返回 KernelError(KindDI) 并附带 diagnostics meta。
func (l *ModuleLoader) validateGraphs() error {
	modules := l.getModulesList()
	nodes := make([]graph.ModuleNodeInput, 0, len(modules))
	for _, mod := range modules {
		if mod == nil || mod.Metadata() == nil {
			continue
		}
		node := graph.ModuleNodeInput{Type: reflect.TypeOf(mod)}
		for _, imp := range mod.Metadata().Imports {
			if imp == nil {
				continue
			}
			node.Imports = append(node.Imports, reflect.TypeOf(imp))
		}
		nodes = append(nodes, node)
	}
	diagnostics := graph.BuildDiagnostics(nodes, l.providers)
	l.mu.Lock()
	l.diagnostics = diagnostics
	l.mu.Unlock()
	if diagnostics == nil {
		return nil
	}
	return &kerrors.KernelError{
		Kind:    kerrors.KindDI,
		Message: diagnostics.String(),
		Meta: map[string]interface{}{
			"moduleCycles":   diagnostics.ModuleCycles,
			"providerCycles": diagnostics.ProviderCycles,
			"stage":          "graph_validation",
		},
	}
}

// hasImportedModule 判断 imports 中是否已包含 target（按 reflect.Type 比较）。
func hasImportedModule(items []Module, target Module) bool {
	targetType := reflect.TypeOf(target)
	for _, item := range items {
		if reflect.TypeOf(item) == targetType {
			return true
		}
	}
	return false
}

// LoadResult 表示一次模块加载完成后的汇总结果。
//
// 它主要给上层拿来读取“这次加载最终收集到了什么”，
// 而不是运行时长期持有的核心状态对象。
type LoadResult struct {
	Modules     []Module
	Providers   map[interface{}]*di.Provider
	Controllers []interface{}
}

// ModuleNode 表示模块树中的一个节点。
//
// 这类结构更适合做可视化、调试或树形遍历，而不是直接参与 DI 解析。
type ModuleNode struct {
	Module   Module
	Children []*ModuleNode
}
