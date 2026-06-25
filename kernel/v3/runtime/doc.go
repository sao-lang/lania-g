// Package runtime 提供框架统一的运行时执行模型。
//
// 它把不同协议先投影成共同抽象，再围绕这套抽象组织出完整执行链路：
// - `HandlerContext`：一次请求/消息/任务触发的统一上下文
// - `Router`：按 `(protocol, method, path)` 组织和匹配 handler
// - `BindingRegistry`：把参数类型解析成运行时值
// - `Executor`：完成路由匹配、DI、binding、参数级 pipe 与 handler 调用
// - `Pipeline`：承接 middleware/guard/interceptor/filter 等 AOP 执行顺序
//
// compiler/adapter 最终都会把产物安装并汇聚到这里，因此 runtime 是协议无关的执行中枢。
package runtime
