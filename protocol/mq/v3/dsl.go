// dsl.go 提供 MQ adapter 的声明式注册 DSL。
package mq

import (
	"fmt"
	"strings"
	"sync"

	coreadapter "github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// Consumer 提供一个全局 DSL 兼容入口，用于声明某个消息消费者：
//
//	mq.Consumer("default", svc).Group("g1").On("topic", svc.Handle).Build()
//
// 它会把声明写入 `core/registry.Global()`。
// 新业务代码更推荐通过 mounted adapter 暴露的 `adapter.API()` 在应用实例上注册声明。
func Consumer(name string, receiver any) *ConsumerBuilder {
	return globalCompatAPI("mq.Consumer").Consumer(name, receiver)
}

// API 是 MQ adapter 的 DSL 入口封装，用于把声明写入 registry。
type API struct {
	reg            *registry.Registry
	fallbackSource string
}

// NewAPI 创建一个 DSL API。
//
// 推荐：使用挂载到应用实例后的 adapter API，或显式传入实例级 registry。
// 兼容：历史上允许 `NewAPI(nil)`，当前等价于 `NewCompatAPI()`。
func NewAPI(reg *registry.Registry) *API {
	if reg == nil {
		return NewCompatAPI()
	}
	return &API{reg: reg}
}

// NewCompatAPI 创建一个显式保留给迁移场景的全局 DSL 入口，不作为新代码默认入口。
func NewCompatAPI() *API {
	return globalCompatAPI("mq.NewCompatAPI()")
}

func globalCompatAPI(source string) *API {
	return &API{reg: registry.Global(), fallbackSource: source}
}

// Consumer 创建一个消息消费者声明构建器。
func (api *API) Consumer(name string, receiver any) *ConsumerBuilder {
	return newConsumerBuilder(name, receiver, api.reg, api.fallbackSource)
}

// ConsumerBuilder 用于构建一个消费者及其订阅声明。
type ConsumerBuilder struct {
	consumer       string
	group          string
	concurrency    int
	receiver       any
	registry       *registry.Registry
	fallbackSource string
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

func newConsumerBuilder(name string, receiver any, reg *registry.Registry, fallbackSource string) *ConsumerBuilder {
	if reg == nil {
		reg = registry.Global()
	}
	return &ConsumerBuilder{
		consumer:       name,
		group:          "",
		concurrency:    1,
		receiver:       receiver,
		registry:       reg,
		fallbackSource: fallbackSource,
		subscriptions:  make([]*SubscriptionBuilder, 0),
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

// Build 完成当前 consumer 声明并写入 registry；忽略构建错误。
func (sb *SubscriptionBuilder) Build() []*SubscriptionDefinition { return sb.consumerBuilder.Build() }

// BuildE 完成当前 consumer 声明并写入 registry；如果声明不合法则返回错误。
func (sb *SubscriptionBuilder) BuildE() ([]*SubscriptionDefinition, error) {
	return sb.consumerBuilder.BuildE()
}

// Build 完成当前消费者声明并写入 registry；忽略构建错误。
func (b *ConsumerBuilder) Build() []*SubscriptionDefinition {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sealed = true
	return b.buildAndRegisterLocked()
}

// BuildE 完成当前消费者声明并写入 registry；如果声明不合法则返回错误。
func (b *ConsumerBuilder) BuildE() ([]*SubscriptionDefinition, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sealed = true
	if err := b.validateLocked(); err != nil {
		b.err = err
		return nil, err
	}
	return b.buildAndRegisterLocked(), nil
}

func (b *ConsumerBuilder) buildAndRegisterLocked() []*SubscriptionDefinition {
	if b.fallbackSource != "" {
		b.registry.RecordFallbackUsage(b.fallbackSource)
	}
	// 这是供 transport 使用的 consumer 级声明
	b.registry.RegisterDecl(AdapterID, "consumers", &ConsumerDefinition{
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
	b.registry.RegisterDecl(AdapterID, "subscriptions", items...)
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
