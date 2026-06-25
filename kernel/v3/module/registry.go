// registry.go 定义模块与编译期 registry 之间的最小耦合接口。
//
// 这个接口本身很薄，但很关键：
// 它让 `ModuleLoader` 可以在不依赖具体模块实现细节的前提下，
// 把当前应用的 registry 传播给需要写声明的模块。
package module

import "github.com/sao-lang/lania-g/kernel/v3/registry"

// RegistryAware 表示模块支持接收编译期 Registry 注入。
//
// ModuleLoader 在装配模块树时会识别这个接口，并把当前 registry 传给模块，
// 让 integration 或模块自身能在 Init 前后写入 binding/声明，
// 而不必把 registry 从应用层手工层层下传。
type RegistryAware interface {
	SetRegistry(*registry.Registry)
}
