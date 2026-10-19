// module.go 把 WebSocket adapter 装配成一个模块。
//
// 这是该协议的唯一入口：把 ws.Module(cfg) 放进根模块的 Imports 即可。
package ws

import (
	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

// Config 描述 WebSocket 协议的装配参数。
type Config struct {
	// Addr 是监听地址，例如 ":8081"。
	Addr string
}

// ProtocolModule 是 WebSocket 协议模块的句柄。
//
// 它既是 module.Module，也暴露该协议的声明 API。
type ProtocolModule interface {
	module.Module
	API() *API
}

// Module 创建 WebSocket 协议模块。
func Module(cfg Config) ProtocolModule {
	a := New(cfg.Addr)
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

// API 返回该协议的声明入口。
func (m *protocolModule) API() *API { return m.adapter.api }

var _ adapter.ModuleAdapter = (*protocolModule)(nil)
