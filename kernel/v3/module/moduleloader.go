// moduleloader.go 实现运行期模块的动态加载、卸载与重建逻辑。
//
// 和静态启动期不同，这一层主要解决的是：
// - 已有模块树在运行中如何增减模块
// - 动态变更后 modules/providers/controllers 索引如何保持一致
// - root imports / moduleRef / loadedModules 三套状态如何同步更新
package module

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
)

// collectLoadedModules 递归遍历模块树，将所有模块写入 loadedModules（按 reflect.Type 去重）。
//
// 该索引用于：
// - 动态 Load/Unload/Reload 时判断模块是否已加载
// - rebuildDynamicState 时全量重建 modules/providers/controllers
func (l *ModuleLoader) collectLoadedModules(mod Module) {
	if mod == nil {
		return
	}

	typeKey := reflect.TypeOf(mod)
	l.mu.Lock()
	l.loadedModules[typeKey] = mod
	l.mu.Unlock()

	if base, ok := mod.(*BaseModule); ok {
		for _, imported := range base.metadata.Imports {
			l.collectLoadedModules(imported)
		}
	}
}

// LoadModule 在运行期动态加载一个 module。
//
// 主要流程：
// - 首次加载时初始化 rootModule/moduleRef
// - 调用 mod.Init() 完成模块初始化与 provider 实例化（非 Request scope）
// - 将模块导出的 token 注入到 root container，便于全局解析
// - 调用 rebuildDynamicState 重建 modules/providers/controllers 索引并校验依赖图
func (l *ModuleLoader) LoadModule(mod Module) error {
	if mod == nil {
		return fmt.Errorf("module is nil")
	}

	typeKey := reflect.TypeOf(mod)
	l.mu.RLock()
	if _, ok := l.loadedModules[typeKey]; ok {
		l.mu.RUnlock()
		return fmt.Errorf("module already loaded: %v", typeKey)
	}
	l.mu.RUnlock()

	if l.rootModule == nil {
		// 首次动态加载时把当前模块视作根模块，
		// 后续新模块会作为 root imports 的增量补充。
		l.rootModule = mod
		l.moduleRef = NewModuleRef(l.rootModule)
	}

	if err := mod.Init(); err != nil {
		return err
	}

	l.mu.Lock()
	l.loadedModules[typeKey] = mod
	l.mu.Unlock()

	if l.rootModule != nil && l.rootModule != mod {
		if rootBase, ok := l.rootModule.(*BaseModule); ok {
			if !hasImportedModule(rootBase.metadata.Imports, mod) {
				rootBase.metadata.Imports = append(rootBase.metadata.Imports, mod)
			}
			for _, exportToken := range exportedTokens(mod) {
				instance, err := mod.Container().Get(exportToken)
				if err != nil {
					continue
				}
				// 动态加载后把新模块导出的 token 也补进 root container，
				// 这样全局解析路径能立刻看到新依赖。
				rootBase.container.RegisterValue(exportToken, instance)
			}
		}
	}

	if l.moduleRef != nil {
		// moduleRef 既服务 scanner/compiler，也服务后续的动态重建。
		l.moduleRef.collectModules(mod)
	}

	return l.rebuildDynamicState()
}

// UnloadModule 在运行期卸载一个 module（按类型 key）。
//
// 注意：它会调用 mod.Destroy()，并把该模块从 root imports/moduleRef 索引中移除，
// 然后重新构建动态状态。
func (l *ModuleLoader) UnloadModule(moduleType interface{}) error {
	typeKey := reflect.TypeOf(moduleType)

	l.mu.RLock()
	mod, ok := l.loadedModules[typeKey]
	if !ok {
		l.mu.RUnlock()
		return fmt.Errorf("module not found: %v", typeKey)
	}
	l.mu.RUnlock()

	if err := mod.Destroy(); err != nil {
		return err
	}

	l.mu.Lock()
	delete(l.loadedModules, typeKey)
	l.mu.Unlock()

	if l.rootModule != nil {
		if rootBase, ok := l.rootModule.(*BaseModule); ok {
			newImports := make([]Module, 0, len(rootBase.metadata.Imports))
			for _, m := range rootBase.metadata.Imports {
				if reflect.TypeOf(m) != typeKey {
					newImports = append(newImports, m)
				}
			}
			rootBase.metadata.Imports = newImports
		}
	}

	if l.moduleRef != nil {
		l.moduleRef.mu.Lock()
		// 动态卸载后显式从 moduleRef 索引删掉，避免 scanner 仍看到旧模块。
		delete(l.moduleRef.modules, typeKey)
		l.moduleRef.mu.Unlock()
	}

	return l.rebuildDynamicState()
}

// ReloadModule 等价于先 Unload 再 Load，并复用原 module 实例。
func (l *ModuleLoader) ReloadModule(moduleType interface{}) error {
	typeKey := reflect.TypeOf(moduleType)

	l.mu.RLock()
	mod, ok := l.loadedModules[typeKey]
	if !ok {
		l.mu.RUnlock()
		return fmt.Errorf("module not found: %v", typeKey)
	}
	l.mu.RUnlock()

	if err := l.UnloadModule(moduleType); err != nil {
		return err
	}

	return l.LoadModule(mod)
}

// IsLoaded 判断某个模块类型是否已被 loader 动态加载并存在于 loadedModules 中。
func (l *ModuleLoader) IsLoaded(moduleType interface{}) bool {
	typeKey := reflect.TypeOf(moduleType)
	l.mu.RLock()
	defer l.mu.RUnlock()
	_, ok := l.loadedModules[typeKey]
	return ok
}

// GetLoadedModules 返回当前所有已动态加载模块的快照列表。
func (l *ModuleLoader) GetLoadedModules() []Module {
	l.mu.RLock()
	defer l.mu.RUnlock()

	modules := make([]Module, 0, len(l.loadedModules))
	for _, mod := range l.loadedModules {
		modules = append(modules, mod)
	}
	return modules
}

// rebuildDynamicState 用于动态加载/卸载后重算索引与依赖图：
// - modules/providers/controllers 全量重建（保证一致性）
// - prepareGlobalModules 注入 global 模块
// - validateGraphs 做循环依赖诊断
func (l *ModuleLoader) rebuildDynamicState() error {
	l.modules = make(map[Module]bool)
	l.providers = make(map[interface{}]*di.Provider)
	l.controllers = make([]interface{}, 0)

	for _, mod := range l.GetLoadedModules() {
		if err := l.loadModule(mod); err != nil {
			return err
		}
	}

	l.prepareGlobalModules()
	return l.validateGraphs()
}
