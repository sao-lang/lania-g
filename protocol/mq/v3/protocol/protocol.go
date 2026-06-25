// package mq 定义消息队列协议在 runtime 中使用的协议标识。
package mq

import "github.com/sao-lang/lania-g/kernel/v3/runtime"

// Protocol 是构建路由键时使用的协议标识：
// 形如 `mq:<Topic>:<Consumer>`，例如 `mq:user.created:default`。
const Protocol runtime.Protocol = "mq"
