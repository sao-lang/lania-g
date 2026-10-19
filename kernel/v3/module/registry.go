// registry.go 定义模块与模块上下文之间的最小耦合接口。
//
// 这个接口本身很薄，但很关键：
// 它让 `ModuleLoader` 可以在不依赖具体模块实现细节的前提下，
// 把当前应用的模块上下文传给模块。
package module

// ModuleContextAware 表示模块支持接收模块上下文注入。
//
// 内置的 BaseModule 已实现该接口（通过 SetModuleContext/Context），
// 只有不嵌入 BaseModule 的自定义模块才需要自行实现。
type ModuleContextAware interface {
	SetModuleContext(*Context)
	Context() *Context
}
