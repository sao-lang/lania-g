// mq_bridge.go 实现 kafka 集成与 MQ adapter 之间的桥接逻辑。
package kafka

import mqadapter "github.com/sao-lang/lania-g/protocol/mq/v3"

// MQConsumerSpec 描述一组需要桥接到 MQ adapter 的 Kafka 消费声明。
type MQConsumerSpec struct {
	Name        string
	Group       string
	Topic       string
	Handler     any
	Concurrency int
}

// RegisterMQConsumers 根据 specs 批量向 MQ adapter 注册 consumer/subscription 定义。
func RegisterMQConsumers(api *mqadapter.API, receiver any, specs ...MQConsumerSpec) []*mqadapter.SubscriptionDefinition {
	if api == nil {
		api = mqadapter.NewCompatAPI()
	}
	builders := make(map[string]*mqadapter.ConsumerBuilder)
	for _, spec := range specs {
		builder := builders[spec.Name]
		if builder == nil {
			builder = api.Consumer(spec.Name, receiver)
			if spec.Group != "" {
				builder.Group(spec.Group)
			}
			if spec.Concurrency > 0 {
				builder.Concurrency(spec.Concurrency)
			}
			builders[spec.Name] = builder
		}
		_ = builder.On(spec.Topic, spec.Handler)
	}
	var out []*mqadapter.SubscriptionDefinition
	for _, builder := range builders {
		out = append(out, builder.Build()...)
	}
	return out
}
