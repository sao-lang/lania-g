// Package grpc 提供 gRPC 协议适配器、声明 DSL 与编译插件。
//
// 它负责收集 gRPC 服务声明、注册协议默认能力，
// 并在应用编译阶段把这些声明转换为 runtime 可执行产物。
package grpc
