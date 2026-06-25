// package httpprotocol 定义 HTTP 协议常量与通用标识。
package httpprotocol

import "github.com/sao-lang/lania-g/kernel/v3/runtime"

const (
	// Protocol 是 HTTP 协议在 runtime 中使用的协议标识。
	Protocol  runtime.Protocol = "http"
	// AllMethod 表示匹配所有 HTTP 方法。
	AllMethod string           = "ALL"
)
