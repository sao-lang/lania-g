package factory

import (
	"fmt"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

// NestFactory 是一个面向装配阶段的应用工厂。
//
// 它适合在“先收集模块、adapter、全局 AOP，再统一构建应用”的场景下使用。
type NestFactory struct {
	modules      []module.Module
	adapters     []adapter.Adapter
	middlewares  []aop.MiddlewareFunc
	guards       []aop.GuardFunc
	interceptors []aop.InterceptorFunc
	pipes        []aop.PipeFunc
	filters      []aop.ExceptionFilterFunc
}

// New 创建新的应用工厂实例（NestFactory）。
//
// NestFactory 用于组装 Application 的输入：
// - root modules（模块树）
// - adapters（协议适配器集合）
// - 全局 AOP（middlewares/guards/interceptors/pipes/filters）
func New() *NestFactory {
	return &NestFactory{
		modules:      make([]module.Module, 0),
		adapters:     make([]adapter.Adapter, 0),
		middlewares:  make([]aop.MiddlewareFunc, 0),
		guards:       make([]aop.GuardFunc, 0),
		interceptors: make([]aop.InterceptorFunc, 0),
		pipes:        make([]aop.PipeFunc, 0),
		filters:      make([]aop.ExceptionFilterFunc, 0),
	}
}

// RegisterModule 追加注册一个 root module。
//
// 当注册多个 module 时，Build 会创建一个聚合 root module 来承载 imports（见 Build）。
func (f *NestFactory) RegisterModule(m module.Module) *NestFactory {
	f.modules = append(f.modules, m)
	return f
}

// UseAdapter 追加注册一个协议适配器（adapter）。
//
// adapter 负责将协议侧能力挂载到 application（例如 HTTP server、gRPC server、GraphQL schema 等）。
func (f *NestFactory) UseAdapter(a adapter.Adapter) *NestFactory {
	f.adapters = append(f.adapters, a)
	return f
}

// UseGlobalMiddleware 追加全局 middleware。
//
// 在 v3 中，这些全局 AOP 会写入 compile-time registry，并在 Compile/Install 后对所有 handler 生效。
func (f *NestFactory) UseGlobalMiddleware(middlewares ...aop.MiddlewareFunc) *NestFactory {
	f.middlewares = append(f.middlewares, middlewares...)
	return f
}

// UseGlobalGuards 追加全局 guard。
func (f *NestFactory) UseGlobalGuards(guards ...aop.GuardFunc) *NestFactory {
	f.guards = append(f.guards, guards...)
	return f
}

// UseGlobalInterceptors 追加全局 interceptor。
func (f *NestFactory) UseGlobalInterceptors(interceptors ...aop.InterceptorFunc) *NestFactory {
	f.interceptors = append(f.interceptors, interceptors...)
	return f
}

// UseGlobalPipes 追加全局 pipe。
func (f *NestFactory) UseGlobalPipes(pipes ...aop.PipeFunc) *NestFactory {
	f.pipes = append(f.pipes, pipes...)
	return f
}

// UseGlobalFilters 追加全局 exception filter。
func (f *NestFactory) UseGlobalFilters(filters ...aop.ExceptionFilterFunc) *NestFactory {
	f.filters = append(f.filters, filters...)
	return f
}

// Build 构建并返回 Application 实例。
//
// 构建步骤：
// 1) 校验至少注册了一个 root module
// 2) 若 root modules > 1，则创建聚合 root module（imports = f.modules）
// 3) 调用 application.NewCompat(root, adapters...) 创建应用实例
// 4) 将全局 AOP 写入 application 的 compile-time registry（由后续 Compile/Install 展开到运行时）
func (f *NestFactory) Build() (*application.Application, error) {
	if len(f.modules) == 0 {
		return nil, fmt.Errorf("no root module registered")
	}

	// v3 中 runtime 不再硬编码任何协议 binding/DSL，由各 adapter 负责挂载与编译。
	root := f.modules[0]
	if len(f.modules) > 1 {
		root = module.CreateModule(f.modules, nil, nil, nil, nil)
	}

	app, err := application.NewCompat(root, f.adapters...)
	if err != nil {
		return nil, err
	}

	// 将全局 AOP 注册到编译期 registry。
	app.UseGlobalMiddlewares(f.middlewares...)
	app.UseGlobalGuards(f.guards...)
	app.UseGlobalInterceptors(f.interceptors...)
	app.UseGlobalPipes(f.pipes...)
	app.UseGlobalFilters(f.filters...)

	return app, nil
}

// Create 快捷创建应用：New + RegisterModule + UseAdapter + Build 的组合。
//
// 当调用方不需要链式组装过程时，可以直接使用这个便捷入口。
func Create(rootModule module.Module, adapters ...adapter.Adapter) (*application.Application, error) {
	factory := New()
	factory.RegisterModule(rootModule)
	for _, adapter := range adapters {
		factory.UseAdapter(adapter)
	}
	return factory.Build()
}
