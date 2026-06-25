package application

import "github.com/sao-lang/lania-g/kernel/v3/registry"

// Registry 是 application 对外暴露的声明注册表类型别名。
//
// 业务侧若只需要创建并传入实例级 registry，优先使用 NewRegistry()，
// 而不是直接依赖 core/registry 包。
type Registry = registry.Registry

// NewRegistry 创建一个新的实例级声明注册表。
//
// 推荐业务接入代码通过 application.NewRegistry() 注入 Application，
// 这样可以把 registry 的创建入口收敛在 application 包内。
func NewRegistry() *Registry {
	return registry.New()
}
