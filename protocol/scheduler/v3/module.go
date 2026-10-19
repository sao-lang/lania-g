// module.go 把 Scheduler adapter 装配成一个模块。
//
// 这是该协议的唯一入口：把 scheduler.Module(cfg) 放进根模块的 Imports 即可。
package scheduler

import (
	"fmt"
	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

// Config 描述 Scheduler 协议的装配参数。
type Config struct {
	// UniqueLocker 为 Unique() 任务提供分布式唯一锁；为空时退化为进程内锁。
	UniqueLocker UniqueLocker
	// HistoryLimit 是每个任务保留的历史运行记录条数，0 表示默认值。
	HistoryLimit int
}

// Module 创建 Scheduler 协议模块。
func Module(cfg Config) module.Module {
	a := New()
	if cfg.UniqueLocker != nil {
		a.SetUniqueLocker(cfg.UniqueLocker)
	}
	if cfg.HistoryLimit > 0 {
		a.SetHistoryLimit(cfg.HistoryLimit)
	}
	m := &protocolModule{BaseModule: module.New(module.Metadata{})}
	m.adapter = a
	return m
}

type protocolModule struct {
	*module.BaseModule
	adapter *Adapter
}

// Adapter 返回模块持有的适配器，供应用在装配阶段收集。
func (m *protocolModule) Adapter() adapter.Adapter { return m.adapter }

var _ adapter.ModuleAdapter = (*protocolModule)(nil)

// Declare 收集实现了 scheduler.Controller 的任务声明。
func (m *protocolModule) Declare(root *module.ModuleRef) error {
	if root == nil || m.adapter == nil || m.adapter.reg == nil {
		return nil
	}
	reg := m.adapter.reg
	reg.ClearDecl(AdapterID, "jobs")
	var errs []error
	err := module.WalkModules(root.GetRootModule(), func(mod module.Module) error {
		metadata := mod.Metadata()
		if metadata == nil {
			return nil
		}
		for _, controller := range metadata.Controllers {
			instance, err := module.ResolveController(mod, controller)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			declarer, ok := instance.(Controller)
			if !ok {
				continue
			}
			routes := newRoutes(reg, instance)
			declarer.Declare(routes)
			if err := routes.Err(); err != nil {
				errs = append(errs, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("scheduler declaration failed: %v", errs)
	}
	return nil
}
