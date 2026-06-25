// Package errors 提供框架内部的“结构化错误”与归一化工具。
//
// 这层的核心目标：
// - 统一错误模型：把任意 error 归一化为 KernelError
// - 统一诊断字段：协议(protocol)、路由(routeKey)、模块(moduleKey)、参数索引(paramIndex)、meta 信息
// - 便于 adapter 做跨协议错误映射：HTTP status / gRPC code / GraphQL error 等
//
// 注意：该包属于框架内部能力，不建议作为应用层的稳定公共 API 直接依赖。
package errors
