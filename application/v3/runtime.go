// runtime.go 实现 Application 的编排名与生命周期。
//
// 启动顺序（Run 内部完成）：
//
//	prepare（声明 + 挂载钩子）→ compile → bootstrap → start adapters → 阻塞
package application

import (
	"errors"
	"fmt"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// Run 编译并启动应用，阻塞到 Stop 被调用。
func (a *Application) Run() error {
	if a == nil {
		return fmt.Errorf("application is nil")
	}
	if err := a.ensureCompiled(); err != nil {
		return err
	}
	if err := a.bootstrapLifecycle(); err != nil {
		return err
	}
	if err := a.emitStartupReport(); err != nil {
		return err
	}
	if err := a.startAdapters(); err != nil {
		return err
	}
	a.waitStop()
	return nil
}

// Stop 按挂载逆序停止适配器，并执行 shutdown 生命周期钩子。
func (a *Application) Stop() error {
	if a == nil {
		return nil
	}
	defer a.stopOnce.Do(func() {
		if a.stopCh != nil {
			close(a.stopCh)
		}
	})
	var firstErr error
	for i := len(a.adapterList) - 1; i >= 0; i-- {
		if err := a.adapterList[i].Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := a.shutdownLifecycle(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (a *Application) waitStop() {
	if a == nil || a.stopCh == nil {
		return
	}
	<-a.stopCh
}

// ensureCompiled 保证声明阶段与编译阶段都已完成。
func (a *Application) ensureCompiled() error {
	if a.compiled != nil {
		return nil
	}
	if !a.prepared {
		if err := a.prepare(); err != nil {
			return err
		}
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

// prepare 执行声明阶段：协议声明与模块挂载钩子。
//
// 声明阶段必须在编译之前完成，这样编译期看到的路由全部来自模块树。
func (a *Application) prepare() error {
	root := a.rootModule()
	declarers := make([]module.Declarer, 0)
	mounters := make([]adapter.HTTPMounterProvider, 0)
	if root != nil {
		if err := module.WalkModules(root, func(mod module.Module) error {
			if declarer, ok := mod.(module.Declarer); ok {
				declarers = append(declarers, declarer)
			}
			if mounter, ok := mod.(adapter.HTTPMounterProvider); ok {
				mounters = append(mounters, mounter)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	if err := module.DeclareAll(a.moduleRef, declarers...); err != nil {
		return err
	}
	if !a.mounted {
		for _, mounter := range mounters {
			if err := mounter.MountHTTP(adapter.HTTPMountRequest{Host: a, Mounter: a}); err != nil {
				return err
			}
		}
		a.mounted = true
	}
	a.prepared = true
	return nil
}

// invalidatePreparation 让下一次 ensureCompiled 重新执行声明阶段。
//
// 热加载新增模块后必须重新收集路由；挂载钩子（如 swagger UI）不重复执行。
func (a *Application) invalidatePreparation() {
	a.prepared = false
	a.compiled = nil
}

func (a *Application) rootModule() module.Module {
	if a == nil || a.moduleRef == nil {
		return nil
	}
	return a.moduleRef.GetRootModule()
}

// buildCompiledRuntime 从模块树 + 声明仓库 + 插件构建 runtime。
func (a *Application) buildCompiledRuntime() (*runtime.Runtime, *compiler.CompiledApp, *compiler.CompileDiagnostics, error) {
	rt := runtime.NewRuntime()
	if a.moduleRef != nil && a.moduleRef.GetContainer() != nil {
		rt.GetExecutor().SetRootContainer(a.moduleRef.GetContainer())
	}
	compiled, err := compiler.Compile(a.moduleRef, a.registry, a.plugins...)
	if err != nil {
		var compileErr *compiler.CompileError
		if errors.As(err, &compileErr) && compileErr.Diagnostics != nil {
			a.lastDiagnostics = compileErr.Diagnostics.Clone()
		}
		return nil, nil, a.LastCompileDiagnostics(), err
	}
	if compiled != nil && compiled.Diagnostics != nil {
		a.lastDiagnostics = compiled.Diagnostics.Clone()
	}
	if err := compiled.Install(rt); err != nil {
		return nil, nil, nil, err
	}
	return rt, compiled, a.LastCompileDiagnostics(), nil
}

func (a *Application) startAdapters() error {
	for _, adp := range a.adapterList {
		if err := adp.Start(); err != nil {
			return err
		}
	}
	return nil
}
