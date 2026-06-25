// defs.go 定义 gRPC adapter 在 registry 与编译阶段使用的声明结构。
package grpc

import (
	"reflect"
)

// RPCMode 描述一条 gRPC 方法的调用模式。
// 框架内部始终以这个字段作为主语义来源，再在 transport 注册阶段
// 派生出底层 `grpc.StreamDesc` 所需的 `ClientStreams/ServerStreams` 标记。
type RPCMode string

const (
	// RPCModeUnary 表示标准 unary RPC。
	RPCModeUnary RPCMode = "unary"
	// RPCModeServerStream 表示服务端流式 RPC。
	RPCModeServerStream RPCMode = "server_stream"
	// RPCModeClientStream 表示客户端流式 RPC。
	RPCModeClientStream RPCMode = "client_stream"
	// RPCModeBidiStream 表示双向流式 RPC。
	RPCModeBidiStream RPCMode = "bidi_stream"
)

// ClientStreams 返回当前模式是否需要在底层 gRPC 注册中打开 client stream 标记。
func (m RPCMode) ClientStreams() bool {
	return m == RPCModeClientStream || m == RPCModeBidiStream
}

// ServerStreams 返回当前模式是否需要在底层 gRPC 注册中打开 server stream 标记。
func (m RPCMode) ServerStreams() bool {
	return m == RPCModeServerStream || m == RPCModeBidiStream
}

// Normalize 将空模式视为 unary，便于兼容旧声明数据。
func (m RPCMode) Normalize() RPCMode {
	if m == "" {
		return RPCModeUnary
	}
	return m
}

// MethodDefinition 是一条 gRPC 方法的编译期声明。
// 它本身不执行业务逻辑，只承载“如何把一个 RPC 编译成 runtime.Handler”的元信息。
type MethodDefinition struct {
	// Service 是声明时填写的 service 名，通常对应 proto service 名，不带前导 `/`。
	Service string
	// Method 是声明时填写的 RPC 方法名。
	Method string
	// Mode 表示当前 RPC 的调用模式；默认值为 unary。
	// 这个字段同时影响：
	// 1. 编译期签名校验规则
	// 2. transport 侧是注册到 `Methods` 还是 `Streams`
	// 3. 运行期 binding 允许注入哪些 wrapper
	Mode RPCMode

	// Receiver 只用于模块 owner 归属解析与 receiver token 推断。
	// 真正执行时不会直接调用这个实例，而是根据编译结果到 DI 容器里解析运行时实例。
	Receiver any
	// HandlerName 是 Go receiver 上实际承接该 RPC 的方法名。
	HandlerName string

	// RequestType 可选地显式钉死请求消息类型（通常是 `*pb.FooRequest`）。
	// 一旦显式给出，transport 就不必再从方法签名中推断。
	// 这在签名不完全遵循约定、或混入 wrapper 参数时很有用。
	// 对 streaming 来说，它只在 unary / server-stream 的“首请求消息”语义下生效。
	RequestType reflect.Type
	// ResponseType 可选地显式钉死响应消息类型。
	// 当前主要用于 streaming 场景下的诊断和未来扩展；
	// 例如后续如果要做更严格的 stream 消息类型检查，可以直接复用这里的元数据。
	ResponseType reflect.Type

	// ParamBindings 记录“参数索引 -> binding 名称”。
	// 主要服务像 `Header[T]` 这种需要 metadata key 的 binding。
	// 这里继续沿用“参数位置 -> 名称”的模型，因此新增 streaming 能力时不需要改 runtime 参数计划结构。
	ParamBindings map[int]string

	// 下面这些 AOP 字段都是纯声明数据；
	// 真正的合并与执行顺序在编译阶段由 plugin 写入 handler.Meta.CompiledAOP。
	Guards       []any
	Interceptors []any
	Middlewares  []any
	Pipes        []any
	ParamPipes   map[int][]any
	Filters      []any
}
