// defs.go 定义 MQ adapter 在 registry 与编译阶段使用的声明结构。
package mq

import (
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
)

// SubscriptionDefinition 表示一条 MQ 订阅 handler 的编译期声明。
// 它描述的是“哪一个 consumer 订阅哪个 topic，并由哪个 receiver 方法处理”。
type SubscriptionDefinition struct {
	Consumer string
	Group    string
	Topic    string

	// Receiver 只用于 owner 归属解析与 receiver token 推断。
	// 真正执行时运行时实例仍然从模块容器里解析。
	Receiver    any
	HandlerName string

	// Concurrency 记录声明期并发意图；是否以及如何落地，由 transport/driver 决定。
	Concurrency int

	// ParamBindings 记录“参数索引 -> binding 名称”。
	// 主要服务像 `Header[T]` 这种需要 header key 的 binding。
	ParamBindings map[int]string
}

// ConsumerDefinition 汇总 transport 需要的 consumer 级别配置。
// 它不会被编译为运行期路由；仅在 adapter.Start() 阶段用于打开 session。
type ConsumerDefinition struct {
	Consumer    string
	Group       string
	Concurrency int
}

// routeOwnership 是 MQ plugin 在 Scan 阶段附加的编译期 owner 信息。
type routeOwnership struct {
	definition    *SubscriptionDefinition
	moduleKey     string
	moduleType    reflect.Type
	receiverToken reflect.Type
	container     *di.Container
}
