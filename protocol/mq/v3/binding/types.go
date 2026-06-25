// types.go 定义 MQ 协议暴露给 handler 的 binding wrapper 与辅助类型。
package mq

import stdctx "context"

// Context 表示标准库的 `context.Context`，可通过 binding 注入到消息处理函数。
// 这样 MQ handler 可以无缝复用取消、超时、trace 等通用 context 语义。
type Context = stdctx.Context

// Message[T] 表示“把消息 payload 解成 T”。
type Message[T any] struct{ Value T }

// Header[T] 表示“从某个消息头 key 里取值并解成 T”。
type Header[T any] struct{ Value T }

// Headers 表示所有消息头的多值映射快照。
type Headers map[string][]string

// Topic 表示当前消息的 topic。
type Topic string

// Consumer 表示当前消息所属的消费者名称（consumer name）。
type Consumer string

// Key 表示当前消息的 key。
type Key string

// RetryCount 表示当前消息的重试次数。
type RetryCount int

// Ack 表示确认消费的回调函数。
// 是否真正提交 offset/ack message，由具体 driver 决定。
type Ack func() error

// Nack 表示拒绝消费的回调函数。
// 某些 driver 可能只是不提交，不一定真的支持“主动退回队列”。
type Nack func(error) error
