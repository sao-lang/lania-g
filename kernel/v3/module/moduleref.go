// moduleref.go 实现模块树在运行期的只读索引视图。
//
// `ModuleLoader` 更偏“构建和维护”；
// `ModuleRef` 则更偏“查询和解析”，给 compiler/runtime/application 提供统一读取入口。
package module

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/sao-lang/lania-g/kernel/v3/di"
)

// ModuleRef 是模块系统的“运行期索引”。
//
// 它持有 root module 的 container，并维护 `reflect.Type -> Module` 的索引，
// 方便 compiler/runtime 在运行期按模块类型或 token 做解析与查找。
type ModuleRef struct {
	module    Module
	container *di.Container
	modules   map[reflect.Type]Module
	mu        sync.RWMutex
}

// ModuleRef 是模块系统的“运行期索引”：
// - 持有 root module 的 container（作为全局解析入口）
// - 按 module 的 reflect.Type 建立索引，便于按类型获取模块或容器
func NewModuleRef(rootModule Module) *ModuleRef {
	ref := &ModuleRef{
		module:  rootModule,
		modules: make(map[reflect.Type]Module),
	}

	if rootModule != nil {
		ref.container = rootModule.Container()
		ref.collectModules(rootModule)
	}

	return ref
}

// collectModules 递归收集模块树，并建立 reflect.Type -> Module 的索引。
//
// 该索引用于：
// - BuildSnapshot 推导模块归属
// - 运行期按类型查找模块/容器（GetModuleContainerByType 等）
func (r *ModuleRef) collectModules(mod Module) {
	if mod == nil {
		return
	}

	typeKey := reflect.TypeOf(mod)
	r.mu.Lock()
	r.modules[typeKey] = mod
	r.mu.Unlock()

	if base, ok := mod.(*BaseModule); ok {
		for _, imported := range base.metadata.Imports {
			r.collectModules(imported)
		}
	}
}

// GetModule 按“模块类型”获取模块实例。
// moduleType 通常传入 `(*MyModule)(nil)` 或 `MyModule{}` 的类型占位。
func (r *ModuleRef) GetModule(moduleType interface{}) (Module, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	targetType := reflect.TypeOf(moduleType)
	if mod, ok := r.modules[targetType]; ok {
		return mod, nil
	}

	// 若没按精确类型命中，再退回到 AssignableTo，
	// 兼容接口类型或不同声明姿势下的模块查找。
	for _, mod := range r.modules {
		if reflect.TypeOf(mod).AssignableTo(targetType) {
			return mod, nil
		}
	}

	return nil, fmt.Errorf("module not found: %v", targetType)
}

// MustGetModule 是 GetModule 的 panic 版本。
func (r *ModuleRef) MustGetModule(moduleType interface{}) Module {
	mod, err := r.GetModule(moduleType)
	if err != nil {
		panic(err)
	}
	return mod
}

// Get 从 root container 解析一个依赖。
// 注意：这里解析的是“root”容器，不是 request-scope child；请求级依赖应由 runtime.Executor 创建 child 后解析。
func (r *ModuleRef) Get(token interface{}) (interface{}, error) {
	if r.container == nil {
		return nil, fmt.Errorf("no container available")
	}
	return r.container.Get(token)
}

// MustGet 是 Get 的 panic 版本。
func (r *ModuleRef) MustGet(token interface{}) interface{} {
	instance, err := r.Get(token)
	if err != nil {
		panic(err)
	}
	return instance
}

// GetContainer 返回 root module 的 container（全局解析入口）。
//
// 对请求级依赖解析而言，通常应在 runtime 中基于它创建 child container，
// 而不是直接把 root container 用作 request scope 容器。
func (r *ModuleRef) GetContainer() *di.Container {
	return r.container
}

// GetAllModules 返回所有已收集模块的列表（快照）。
func (r *ModuleRef) GetAllModules() []Module {
	r.mu.RLock()
	defer r.mu.RUnlock()

	modules := make([]Module, 0, len(r.modules))
	for _, mod := range r.modules {
		modules = append(modules, mod)
	}
	return modules
}

// GetAllModulesWithTypes 返回 reflect.Type -> Module 的映射快照。
func (r *ModuleRef) GetAllModulesWithTypes() map[reflect.Type]Module {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(map[reflect.Type]Module, len(r.modules))
	for typ, mod := range r.modules {
		out[typ] = mod
	}
	return out
}

// GetModuleContainerByType 按模块 reflect.Type 获取其 DI container。
//
// 返回 (container, true) 表示命中；否则返回 (nil, false)。
func (r *ModuleRef) GetModuleContainerByType(moduleType reflect.Type) (*di.Container, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mod, ok := r.modules[moduleType]
	if !ok || mod == nil {
		return nil, false
	}
	return mod.Container(), true
}

// GetRootModule 返回 root module。
func (r *ModuleRef) GetRootModule() Module {
	return r.module
}
