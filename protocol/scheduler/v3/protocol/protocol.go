// package scheduler 定义调度协议在 runtime 中使用的协议标识。
package scheduler

import "github.com/sao-lang/lania-g/kernel/v3/runtime"

// Protocol 是构建路由键时使用的协议标识：
// 形如 `scheduler:<TriggerType>:<JobName>`。
const Protocol runtime.Protocol = "scheduler"
