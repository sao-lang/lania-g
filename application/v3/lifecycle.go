// lifecycle.go 实现 Application 级别的 bootstrap/shutdown 生命周期调度。
//
// 它并不直接决定模块如何初始化；
// 真正的 provider/controller 实例化发生在 module.Init。
// 这里负责的是“应用已经装起来之后，再按模块拓扑调用应用级生命周期钩子”。
package application

import (
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/graph"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// bootstrapLifecycle 只会成功执行一次，之后由 `bootstrapped` 防重复。
func (a *Application) bootstrapLifecycle() error {
	if a.bootstrapped {
		return nil
	}
	if err := a.forEachModuleLifecycle(false, func(instance any) error {
		if hook, ok := instance.(module.OnApplicationBootstrap); ok {
			return hook.OnApplicationBootstrap()
		}
		return nil
	}); err != nil {
		return err
	}
	a.bootstrapped = true
	return nil
}

// shutdownLifecycle 在应用已 bootstrap 的前提下，按逆拓扑顺序触发 shutdown。
func (a *Application) shutdownLifecycle() error {
	if !a.bootstrapped {
		return nil
	}
	err := a.forEachModuleLifecycle(true, func(instance any) error {
		if hook, ok := instance.(module.OnApplicationShutdown); ok {
			return hook.OnApplicationShutdown()
		}
		return nil
	})
	if err == nil {
		a.bootstrapped = false
	}
	return err
}

func (a *Application) forEachModuleLifecycle(reverse bool, visit func(instance any) error) error {
	modules := a.moduleLoader.GetLoadedModules()
	return a.forEachModulesLifecycle(modules, reverse, visit)
}

func (a *Application) forEachSubtreeLifecycle(root module.Module, reverse bool, visit func(instance any) error) error {
	moduleByType := make(map[reflect.Type]module.Module)
	a.collectModuleTree(root, moduleByType)
	modules := make([]module.Module, 0, len(moduleByType))
	for _, mod := range moduleByType {
		modules = append(modules, mod)
	}
	return a.forEachModulesLifecycle(modules, reverse, visit)
}

// forEachModulesLifecycle 按模块拓扑顺序遍历模块内的非 request-scope provider 实例。
// 这是应用级 lifecycle hook 的真正执行引擎。
func (a *Application) forEachModulesLifecycle(modules []module.Module, reverse bool, visit func(instance any) error) error {
	nodes := make([]graph.ModuleNodeInput, 0, len(modules))
	moduleByType := make(map[reflect.Type]module.Module, len(modules))
	for _, mod := range modules {
		if mod == nil || mod.Metadata() == nil {
			continue
		}
		t := reflect.TypeOf(mod)
		moduleByType[t] = mod
		node := graph.ModuleNodeInput{Type: t}
		for _, imp := range mod.Metadata().Imports {
			if imp != nil {
				node.Imports = append(node.Imports, reflect.TypeOf(imp))
			}
		}
		nodes = append(nodes, node)
	}
	order, err := graph.NewModuleGraph(nodes).TopologicalSort()
	if err != nil {
		return err
	}
	if reverse {
		for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
			order[i], order[j] = order[j], order[i]
		}
	}
	for _, moduleType := range order {
		mod := moduleByType[moduleType]
		if mod == nil || mod.Container() == nil {
			continue
		}
		providers := mod.Container().GetAll()
		for token, provider := range providers {
			if provider != nil && provider.Scope == di.Request {
				// request-scope provider 没有稳定的应用级生命周期，不参与 bootstrap/shutdown。
				continue
			}
			instance, getErr := mod.Container().Get(token)
			if getErr != nil {
				continue
			}
			if aware, ok := instance.(interface{ SetRegistry(*registry.Registry) }); ok {
				// lifecycle 前补齐 registry/moduleRef，让感知型 integration 能拿到当前应用上下文。
				aware.SetRegistry(a.registry)
			}
			if aware, ok := instance.(interface{ SetModuleRef(*module.ModuleRef) }); ok {
				aware.SetModuleRef(a.moduleRef)
			}
			if err := visit(instance); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *Application) collectModuleTree(mod module.Module, moduleByType map[reflect.Type]module.Module) {
	if mod == nil {
		return
	}
	t := reflect.TypeOf(mod)
	if _, exists := moduleByType[t]; exists {
		return
	}
	moduleByType[t] = mod
	if mod.Metadata() == nil {
		return
	}
	for _, imported := range mod.Metadata().Imports {
		a.collectModuleTree(imported, moduleByType)
	}
}
