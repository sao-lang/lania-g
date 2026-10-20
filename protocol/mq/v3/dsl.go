// dsl.go 提供 MQ adapter 的声明式注册 DSL。
package mq

import (
	"fmt"
	"strings"
	"sync"

	coreadapter "github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// Controller 由需要声明 MQ 消费者的控制器实现。
type Controller interface {
	Declare(*Routes)
}

// Routes 收集一个控制器声明的全部 MQ 消费者。
//
// 它是 MQ 唯一的声明入口：控制器在所属模块内完成注册，无需手动 Build。
type Routes struct {
	reg      *registry.Registry
	receiver any
	current  *ConsumerBuilder
	err      error
}

// NewRoutes 创建一个绑定到 reg 的声明收集器。
//
// 正常装配由 adapter 在 Declare 阶段构造；测试或自定义装载器可直接使用。
func NewRoutes(reg *registry.Registry, receiver any) *Routes {
	return &Routes{reg: reg, receiver: receiver}
}

// Consumer 开始声明一个消费者。
func (r *Routes) Consumer(name string) *ConsumerBuilder {
	if err := r.sealCurrent(); err != nil && r.err == nil {
		r.err = err
	}
	b := newConsumerBuilder(name, r.receiver, r)
	r.current = b
	return b
}

// Err 结束声明并返回累计错误。
func (r *Routes) Err() error {
	if r == nil {
		return nil
	}
	if err := r.sealCurrent(); err != nil && r.err == nil {
		r.err = err
	}
	return r.err
}

func (r *Routes) sealCurrent() error {
	if r == nil || r.current == nil {
		return nil
	}
	b := r.current
	r.current = nil
	return b.seal()
}

// ConsumerBuilder 用于构建一个消费者及其订阅声明。
type ConsumerBuilder struct {
	consumer       string
	group          string
	concurrency    int
	receiver       any
	routes         *Routes
	subscriptions  []*SubscriptionBuilder
	sealed         bool
	err            error
	mu             sync.RWMutex
}

// SubscriptionBuilder 用于构建某个 topic 的订阅声明。
type SubscriptionBuilder struct {
	consumerBuilder *ConsumerBuilder
	topic           string
	handler         any
	handlerName     string
	paramBindings   map[int]string
}

func newConsumerBuilder(name string, receiver any, routes *Routes) *ConsumerBuilder {
	return &ConsumerBuilder{
		consumer:       name,
		group:          "",
		concurrency:    1,
		receiver:      receiver,
		routes:        routes,
		subscriptions: make([]*SubscriptionBuilder, 0),
	}
}

// Group 设置 consumer group。
func (b *ConsumerBuilder) Group(group string) *ConsumerBuilder {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sealed {
		return b
	}
	b.group = group
	return b
}

// Concurrency 设置当前消费者的并发度。
func (b *ConsumerBuilder) Concurrency(n int) *ConsumerBuilder {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sealed {
		return b
	}
	if n > 0 {
		b.concurrency = n
	}
	return b
}

// On 声明某个 topic 的订阅处理函数。
func (b *ConsumerBuilder) On(topic string, handler any) *SubscriptionBuilder {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sealed {
		return nil
	}
	sb := &SubscriptionBuilder{
		consumerBuilder: b,
		topic:           topic,
		handler:         handler,
		handlerName:     coreadapter.FindMethodName(b.receiver, handler),
		paramBindings:   make(map[int]string),
	}
	b.subscriptions = append(b.subscriptions, sb)
	return sb
}

// BindParam 为某个参数索引绑定一个名字，供编译期/运行期参数解析使用。
func (sb *SubscriptionBuilder) BindParam(paramIndex int, name string) *SubscriptionBuilder {
	if name == "" {
		return sb
	}
	sb.paramBindings[paramIndex] = name
	return sb
}

// On 在同一个 consumer 下继续声明一个 topic 订阅。
func (sb *SubscriptionBuilder) On(topic string, handler any) *SubscriptionBuilder {
	return sb.consumerBuilder.On(topic, handler)
}

// seal 完成当前消费者声明并写入 registry。
func (b *ConsumerBuilder) seal() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sealed {
		return b.err
	}
	b.sealed = true
	if err := b.validateLocked(); err != nil {
		b.err = err
		return err
	}
	b.buildAndRegisterLocked()
	return nil
}

func (b *ConsumerBuilder) buildAndRegisterLocked() []*SubscriptionDefinition {
	// 这是供 transport 使用的 consumer 级声明
	b.routes.reg.RegisterDecl(AdapterID, "consumers", &ConsumerDefinition{
		Consumer:    b.consumer,
		Group:       b.group,
		Concurrency: b.concurrency,
	})

	defs := make([]*SubscriptionDefinition, 0, len(b.subscriptions))
	for _, sb := range b.subscriptions {
		defs = append(defs, &SubscriptionDefinition{
			Consumer:      b.consumer,
			Group:         b.group,
			Topic:         sb.topic,
			Receiver:      b.receiver,
			HandlerName:   sb.handlerName,
			Concurrency:   b.concurrency,
			ParamBindings: coreadapter.CopyIntStringMap(sb.paramBindings),
		})
	}
	items := make([]any, 0, len(defs))
	for _, def := range defs {
		items = append(items, def)
	}
	b.routes.reg.RegisterDecl(AdapterID, "subscriptions", items...)
	return defs
}

// Err 返回 DSL 构建过程中记录下来的错误。
func (b *ConsumerBuilder) Err() error { return b.err }

func (b *ConsumerBuilder) validateLocked() error {
	if strings.TrimSpace(b.consumer) == "" {
		return fmt.Errorf("mq consumer name is required")
	}
	if b.receiver == nil {
		return fmt.Errorf("mq consumer receiver is nil")
	}
	if len(b.subscriptions) == 0 {
		return fmt.Errorf("mq consumer %s has no subscriptions", b.consumer)
	}
	for _, sb := range b.subscriptions {
		if sb == nil {
			return fmt.Errorf("mq subscription builder is nil")
		}
		if strings.TrimSpace(sb.topic) == "" {
			return fmt.Errorf("mq subscription topic is required")
		}
		if sb.handler == nil || strings.TrimSpace(sb.handlerName) == "" {
			return fmt.Errorf("invalid mq subscription declaration: consumer=%s topic=%s", b.consumer, sb.topic)
		}
	}
	return nil
}
