// builder.go 定义协议侧向 runtime 注册处理器的最小接口。
//
// 这个文件很小，但它把“协议适配器如何把自己的路由体系接进 runtime”
// 抽象成了一个极薄边界。
package runtime

// HandlerBuilder 是协议侧向 runtime 注册 handler 的最小接口。
//
// 各协议适配器（HTTP、WebSocket、gRPC、GraphQL 等）只要实现它，
// 就能把自己的路由/匹配逻辑接入统一的 runtime.Router。
type HandlerBuilder interface {
	// Register 向 Router 注册当前协议的处理器定义。
	Register(router *Router) error
}
