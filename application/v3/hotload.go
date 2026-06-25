// hotload.go 实现运行期动态加载模块并原位替换编译/runtime 状态的能力。
//
// HotLoad 的关键不是“把模块塞进来”本身，
// 而是要保证失败时能回滚，成功时能让已挂载 adapter 感知新的 runtime。
package application

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// HotLoad 动态加载一个新模块，重新编译应用，并原位替换 runtime。
// 支持热刷新 runtime 的 adapter 可以通过实现 `Reload() error` 感知这次变更。
func (a *Application) HotLoad(mod module.Module) error {
	if a == nil {
		return fmt.Errorf("application is nil")
	}
	if mod == nil {
		return fmt.Errorf("module is nil")
	}
	if a.moduleLoader == nil {
		return fmt.Errorf("module loader is nil")
	}
	if a.moduleLoader.IsLoaded(mod) {
		return fmt.Errorf("module already loaded: %v", reflect.TypeOf(mod))
	}
	if err := a.moduleLoader.LoadModule(mod); err != nil {
		return err
	}

	previous := a.snapshotState()
	a.moduleRef = a.moduleLoader.GetModuleRef()
	rt, compiled, diag, err := a.buildCompiledRuntime()
	if err != nil {
		// 热加载失败时尽量恢复到旧状态，避免应用卡在“半新半旧”的中间态。
		_ = a.moduleLoader.UnloadModule(mod)
		a.restoreState(previous)
		return err
	}
	if a.bootstrapped {
		if err := a.forEachSubtreeLifecycle(mod, false, func(instance any) error {
			if hook, ok := instance.(module.OnApplicationBootstrap); ok {
				return hook.OnApplicationBootstrap()
			}
			return nil
		}); err != nil {
			// 新模块 bootstrap 失败同样回滚，保证热加载的原子性尽量接近“全有或全无”。
			_ = a.moduleLoader.UnloadModule(mod)
			a.restoreState(previous)
			return err
		}
	}

	a.runtime = rt
	a.compiled = compiled
	a.lastDiagnostics = diag
	return a.reloadHotAdapters()
}

// reloadHotAdapters 通知支持热刷新的 adapter 重新读取当前 runtime/route 状态。
func (a *Application) reloadHotAdapters() error {
	for _, adp := range a.adapterList {
		reloadable, ok := adp.(interface{ Reload() error })
		if !ok {
			continue
		}
		if err := reloadable.Reload(); err != nil {
			return err
		}
	}
	return nil
}

// appStateSnapshot 保存 HotLoad 回滚时需要恢复的最小应用状态。
type appStateSnapshot struct {
	moduleRef       *module.ModuleRef
	runtime         *runtime.Runtime
	compiled        *compiler.CompiledApp
	lastDiagnostics *compiler.CompileDiagnostics
}

func (a *Application) snapshotState() appStateSnapshot {
	return appStateSnapshot{
		moduleRef:       a.moduleRef,
		runtime:         a.runtime,
		compiled:        a.compiled,
		lastDiagnostics: a.lastDiagnostics,
	}
}

func (a *Application) restoreState(snapshot appStateSnapshot) {
	a.moduleRef = snapshot.moduleRef
	a.runtime = snapshot.runtime
	a.compiled = snapshot.compiled
	a.lastDiagnostics = snapshot.lastDiagnostics
}
