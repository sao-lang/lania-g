// driver.go 定义 MQ adapter 的底层驱动抽象与公共约束。
package mq

import stdctx "context"

// Message 是 driver 层投递给 adapter 的传输对象。
// driver 尽量把 topic/key/header/body/ack/nack 等信息填全，adapter 再统一投影到 runtime。
type Message struct {
	Topic string
	Key   string

	Headers map[string][]string
	Body    []byte
	Raw     any

	RetryCount int

	Ack  func() error
	Nack func(error) error
}

// ConsumerConfig 描述一个逻辑 consumer 实例该如何打开。
// 它不包含 runtime handler，只描述 transport 需要连接和订阅哪些 topic。
type ConsumerConfig struct {
	Consumer string
	Group    string
	Topics   []string
}

// Session 表示一个已打开的 MQ 消费会话。
// adapter 会持续调用 Poll 拉取消息，直到 context 取消或 Session.Close。
type Session interface {
	Poll(ctx stdctx.Context) (*Message, error)
	Close() error
}

// Driver 是 MQ transport 的抽象边界。
// 不同消息系统只需要实现 Open/Poll/Close，就能复用上层的 runtime/binding/DSL 体系。
type Driver interface {
	Open(ctx stdctx.Context, cfg ConsumerConfig) (Session, error)
}
