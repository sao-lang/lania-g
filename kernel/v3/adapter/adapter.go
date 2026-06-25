// adapter.go 定义协议适配器与宿主之间的通用契约。
package adapter

import (
	"net/http"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// Host 由 v3 的 application 实现，而不是由 core runtime 实现。
// Adapter 通过它来：
// - 向 runtime 注册默认 binding
// - 读取 Registry 中保存的声明
// - 获取 DI 容器与 ModuleRef，供后续请求期实例构造使用
type Host interface {
	Runtime() *runtime.Runtime
	Registry() *registry.Registry
	ModuleRef() *module.ModuleRef
}

// Adapter 表示一个可挂载的协议适配器/协议插件。
// 它是 application 与具体协议实现之间的统一边界。
//
// 一般来说，一个 adapter 会同时负责三件事：
// - 暴露 DSL/API，把业务声明写入 registry
// - 提供 protocol plugin，把声明编译成 runtime 可执行路由
// - 启动/停止底层 transport，例如 HTTP server、gRPC server、消息消费循环等
type Adapter interface {
	ID() string
	Mount(host Host) error
	Start() error
	Stop() error
	API() any
}

// SharedListenerConfigurator 由那些需要共享监听地址的 adapter 实现。
// 典型场景是 HTTP 这种“可以由应用统一决定监听地址”的协议。
//
// 当应用调用 `app.Listen(addr)` 时，这类 adapter 会先通过这里接收地址，
// 再在各自的 Start 阶段真正启动监听。
type SharedListenerConfigurator interface {
	RequiresSharedListen() bool
	ConfigureSharedListen(addr string) error
}

// HTTPMountHost 允许某个 adapter 把 `net/http` handler 挂到共享 HTTP 监听器上，
// 同时又不直接依赖某个具体的 HTTP adapter 实现。
//
// 它通常由 Application（或类似宿主）实现，底层再委托给已经挂载的 HTTP adapter。
type HTTPMountHost interface {
	MountHTTP(pattern string, handler http.Handler) error
}
