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
//	root := module.New(module.Metadata{
//		Imports:     []module.Module{http.Module(http.Config{Addr: ":8080"})},
//		Controllers: []any{&UserController{}},
//	})
//	app, err := application.New(root, application.Options{Logger: os.Stdout})
//	if err != nil {
//		return err
//	}
//	return app.Run()
//
// 两个入口的语义：
// - `New` 加载模块树、发现并挂载 adapter（协议以模块形式出现在 Imports 中）
// - `Run` 依次执行「声明（Declare）→ 编译 → 启动 adapter → 阻塞」
//
// `Diagnostics()` 可在启动后返回最近一次编译诊断快照。
//
// 注意：
// - 协议 DSL（例如 HTTP Controller、GraphQL Resolver DSL）不在本包中，
//   而是在各自的 `adapter/*` 包中声明
// - `core/compiler`、`core/runtime` 等属于内部基础设施；业务代码优先依赖
//   `application`、`adapter/*`、`binding/*` 与 `integrations/*`
package application
