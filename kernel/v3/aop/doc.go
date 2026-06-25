// Package aop 定义框架运行时使用的横切能力抽象。
//
// 这里的类型主要服务于 runtime pipeline：
// - `Middleware` 负责在进入 handler 前做通用前置处理
// - `Guard` 决定当前请求是否允许继续执行
// - `Interceptor` 包裹 handler 调用，可改写入参、结果和错误
// - `Pipe` 负责参数或数据转换/校验
// - `ExceptionFilter` 负责消费错误或 panic
//
// 整个包尽量只暴露与协议无关的抽象，通过 `ExecutionContext` 把 runtime 信息投影出来，
// 这样 HTTP、GraphQL、gRPC、MQ 等协议都能复用同一套 AOP 机制。
package aop
