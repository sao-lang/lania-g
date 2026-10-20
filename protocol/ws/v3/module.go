// module.go 把 WebSocket adapter 装配成一个模块。
//
// 这是该协议的唯一入口：把 ws.Module(cfg) 放进根模块的 Imports 即可。
package ws

import (
	"fmt"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

// Config 描述 WebSocket 协议的装配参数。
type Config struct {
	// Addr 是监听地址，例如 ":8081"。
	Addr string
	// SocketPath 是 Socket.IO 的挂载路径，默认 /socket.io/。
	SocketPath string
	// ResponseEventSuffix 是成功响应的事件后缀。
	ResponseEventSuffix string
	// ErrorEventSuffix 是错误响应的事件后缀。
	ErrorEventSuffix string
}

// Module 创建 WebSocket 协议模块。
func Module(cfg Config) module.Module {
	a := New(cfg.Addr)
	if strings.TrimSpace(cfg.SocketPath) != "" {
		a = a.WithSocketPath(cfg.SocketPath)
	}
	if cfg.ResponseEventSuffix != "" {
		a = a.WithResponseEventSuffix(cfg.ResponseEventSuffix)
	}
	if cfg.ErrorEventSuffix != "" {
		a = a.WithErrorEventSuffix(cfg.ErrorEventSuffix)
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

// Declare 收集实现了 ws.Controller 的 gateway 声明。
func (m *protocolModule) Declare(root *module.ModuleRef) error {
	if root == nil || m.adapter == nil || m.adapter.reg == nil {
		return nil
	}
	reg := m.adapter.reg
	reg.ClearDecl(AdapterID, "handlers")
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
			routes := NewRoutes(reg, instance)
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
		return fmt.Errorf("ws declaration failed: %v", errs)
	}
	return nil
}
