// package grpc 定义 gRPC 协议在 runtime 中使用的协议标识。
package grpc

import "github.com/sao-lang/lania-g/kernel/v3/runtime"

// Protocol 是构建路由键时使用的协议标识：
// 形如 `grpc:<Method>:<Service>`，例如 `grpc:Echo:UserService`。
const Protocol runtime.Protocol = "grpc"
