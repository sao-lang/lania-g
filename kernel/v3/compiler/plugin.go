// plugin.go 定义协议编译插件的最小接口和编译产物结构。
//
// `compile.go` 负责调度这套接口，
// 各具体协议则只需要实现 `Register/Scan/Compile` 三步即可接入主编译流程。
package compiler

import (
	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// ProtocolPlugin 是“协议编译流水线”的扩展点。
// 一个协议插件通常负责：
// - 向 `core/registry` 注册默认声明或默认 binding
// - 扫描声明并解析模块归属
// - 把声明编译为该协议专属的 matcher/routes/handlers
// - 将编译产物安装到 runtime
//
// 通过这个接口，新增协议通常不需要直接修改 core 主流程。
type ProtocolPlugin interface {
	ID() string
	Protocol() runtime.Protocol

	// Register 会在编译前调用。
	// 插件应在这里向 registry 注册默认 binding 与其他编译期默认能力。
	Register(reg *registry.Registry)

	Scan(moduleRef *module.ModuleRef, reg *registry.Registry) (any, error)
	Compile(scan any, reg *registry.Registry, global registry.GlobalAOPRegistration) (*CompiledProtocol, error)
}

// CompiledProtocol 表示某个协议插件编译完成后的产物集合。
// 它是插件交给 `CompiledApp.Install()` 的安装单元。
type CompiledProtocol struct {
	Protocol        runtime.Protocol
	Routes          map[string]*runtime.Handler
	RouteContainers map[string]*di.Container

	// Install 负责把该协议的编译产物真正安装到 runtime.Router 中。
	// 协议若需要 matcher，也应在这里一并装入 router。
	Install func(router *runtime.Router) error
}
