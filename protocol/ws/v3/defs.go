// defs.go 定义 WS adapter 在 registry 与编译阶段使用的声明结构。
package ws

// HandlerDefinition 表示一条 WS 事件处理器的编译期声明。
// 它描述的是：
// - 哪个 namespace（Prefix）
// - 哪个 event
// - 由哪个 gateway 的哪个方法处理
//
// plugin 编译时会再把这些纯声明数据转成 routeKey、AOP plan 和 runtime.Handler。
type HandlerDefinition struct {
	Prefix     string
	Event      string
	Gateway    any
	MethodName string

	// 下面这些字段是编译期收集的 AOP 声明，
	// 运行时会统一被编译进 handler.Meta.CompiledAOP。
	Guards       []any
	Interceptors []any
	Middlewares  []any
	Pipes        []any
	ParamPipes   map[int][]any
	Filters      []any
}
