// Package http 提供 HTTP 协议适配器、声明 DSL 与运行时桥接能力。
//
// 如果你是第一次接触 `lania-g/v3`，可以把这个包理解为两部分：
// - adapter：负责把 HTTP 服务挂到 Application/Runtime 上并对外监听
// - DSL：负责把“控制器/路由/AOP/文档”这些声明写入 registry，供编译阶段读取
//
// 最常见的声明方式是 Controller DSL：
//
//	builder := http.Controller("/users", &UserController{})
//	builder.UseGuards(AuthGuard{})
//	builder.Get("/:id", (*UserController).GetByID)
//	builder.Post("/", (*UserController).Create)
//	builder.Build()
//
// 初学者可以先记住这一点：
// - `Controller(...).Get/Post/...` 写的是“声明”，不是立刻启动 HTTP 服务
// - 真正把这些声明变成可执行路由，发生在 `application.Application` 的 Compile/Install 阶段
// - 最后由挂载到 Application 的 HTTP adapter 启动监听并把请求转进 runtime
//
// 一个常见的调用顺序是：
//
//	声明路由 -> application.NewWithOptions(..., application.Options{Registry: application.NewRegistry()}, ...) -> CompileDiagnostics() -> Listen(":8080")
//
// 另外，controller 级别的 `UseGuards/UsePipes/UseInterceptors/...` 应在声明第一条路由前调用；
// 一旦开始 `Get/Post/...`，后续再改 controller 级配置会返回错误。
package http
