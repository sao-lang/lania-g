// Package application 提供 `lania-g/v3` 的应用装配入口。
//
// 对首次接触 v3 的使用者来说，可以把它理解为“启动总控”：
// - 接收 root module，并通过 ModuleLoader 构建模块树与 DI 容器
// - 挂载 adapter，收集各协议的编译插件与运行时能力
// - 触发编译，把 registry 中的声明转换为可安装到 runtime 的产物
// - 启动生命周期，并驱动 adapter 对外提供 HTTP/gRPC/WS/MQ 等能力
//
// 一条最重要的主线是：
//
//	Module
//	  -> Application
//	  -> Adapter 挂载
//	  -> Compile / Install
//	  -> Runtime Execute
//
// 推荐的最小使用方式：
//
//	app, err := application.NewWithOptions(rootModule, application.Options{
//		Registry: application.NewRegistry(),
//	}, httpAdapter)
//	if err != nil {
//		return err
//	}
//	if _, err := app.CompileDiagnostics(); err != nil {
//		return err
//	}
//	return app.Listen(":8080")
//
// 常见区别：
// - `NewWithOptions` 是推荐入口，可显式注入实例级 registry
// - `New` 是便捷入口，仍保留历史兼容的默认回退语义
// - `CompileDiagnostics` 适合在启动前做“声明是否可编译”的预检查
// - `Run` 使用各 adapter 自己的监听配置启动
// - `Listen` 为需要共享监听地址的 adapter 提供统一 addr
//
// 注意：
// - 协议 DSL（例如 HTTP Controller、GraphQL Resolver DSL）不在本包中，
//   而是在各自的 `adapter/*` 包中声明
// - `core/compiler`、`core/runtime` 等属于内部基础设施；业务代码优先依赖
//   `application`、`adapter/*`、`binding/*` 与 `integrations/*`
package application
