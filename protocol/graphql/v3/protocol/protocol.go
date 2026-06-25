// package graphql 定义 GraphQL 协议在 runtime 中使用的协议标识。
package graphql

import "github.com/sao-lang/lania-g/kernel/v3/runtime"

// Protocol 是构建路由键时使用的协议标识：
// 例如 `graphql:Query.user` / `graphql:Mutation.createUser`。
const Protocol runtime.Protocol = "graphql"
