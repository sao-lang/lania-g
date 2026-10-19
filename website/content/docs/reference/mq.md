---
title: MQ
weight: 10
prev: /docs/reference/websocket
next: /docs/reference/scheduler
---

包：`github.com/sao-lang/lania-g/protocol/mq/v3`（Kafka 驱动：`.../mq/v3/kafka`）

## 1. 驱动契约

MQ adapter 只依赖三个动作：`Open` / `Poll` / `Close`。

```go
type Driver interface {
	Open(ctx context.Context, cfg ConsumerConfig) (Session, error)
}

type Session interface {
	Poll(ctx context.Context) (*Message, error)
	Close() error
}

type ConsumerConfig struct {
	Consumer string
	Group    string
	Topics   []string
}

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
```

实现 `Driver` 即可接入任意消息系统（RocketMQ / NATS / Redis Stream / 内存驱动）。

## 2. 模块入口

```go
func Module(cfg Config) module.Module

type Config struct {
	Driver Driver  // 例如 mqkafka.New(...)
}
```

```go
root := module.New(module.Metadata{
	Imports: []module.Module{
		mqadapter.Module(mqadapter.Config{
			Driver: mqkafka.New(mqkafka.Config{Brokers: []string{"127.0.0.1:9092"}}),
		}),
	},
	Controllers: []any{&Consumer{}},
})
```

启动流程：

```text
collectConsumerConfigs(registry)   // consumers + subscriptions 合并成 topic 列表
  → driver.Open(cfg) → Session
  → go consumeLoop(ctx, cfg, session)
      → session.Poll
      → dispatchMessage（构造 HandlerContext → runtime.Execute）
      → 成功 ack / 失败 nack
```

## 3. 消费者声明

```go
type Controller interface {
	Declare(*Routes)
}
```

```go
func (c *Consumer) Declare(r *mq.Routes) {
	r.Consumer("default").
		Group("order-group").
		Concurrency(4).
		On("order.created", c.OnOrder).
			BindParam(3, "X-Trace-Id").
		On("order.paid", c.OnPaid)
}
```

| 类型 | 方法 |
|------|------|
| `Routes` | `Consumer(name) *ConsumerBuilder` |
| `ConsumerBuilder` | `Group(string)` / `Concurrency(n)` |
| | `On(topic, handler)` / `Err()`（自动 seal） |
| `SubscriptionBuilder` | `BindParam(index, name)`（`Header[T]` 必需） |

消费者放进模块的 `Controllers` 槽位，声明阶段自动收集。

> `Concurrency` 只写入声明，**当前版本的 adapter 每个 consumer 只跑一个 consumeLoop**，并发由 driver 实现决定（例如 kafka-go 的 reader 批量拉取）。

## 4. 处理器签名

```go
type orderPayload struct {
	ID string `json:"id"`
}

func (c *Consumer) OnOrder(
	ctx stdctx.Context,
	payload mqbinding.Message[orderPayload],
	headers mqbinding.Headers,
	trace mqbinding.Header[string],
	topic mqbinding.Topic,
	retry mqbinding.RetryCount,
	ack mqbinding.Ack,
	nack mqbinding.Nack,
) error {
	// 需要显式拒绝时手动调用 nack
	return nil
}
```

| wrapper | 含义 |
|---------|------|
| `Context` | `context.Context` |
| `Message[T]` | 消息体（JSON 解码） |
| `Header[T]` | 单个 header（需 `BindParam`） |
| `Headers` | 全部 header（key 已小写） |
| `Topic` / `Consumer` / `Key` | 投递元信息 |
| `RetryCount` | 重试次数 |
| `Ack` / `Nack` | 确认 / 拒绝 |

## 5. 确认语义

| 场景 | 行为 |
|------|------|
| handler 返回 nil | adapter 自动 `Ack()` |
| handler 返回 error | adapter 自动 `Nack(err)` |
| 业务提前调用 `ack()` | 有效；`once` 包装保证不会重复提交 |

`Ack` / `Nack` 都被 `sync.Once` 包装，重复调用是安全的空操作。

Kafka 驱动实现：

- `Ack` → `CommitMessages`（同一条消息最多提交一次）
- `Nack` → 目前**不提交 offset**，是否重投交给 broker/group rebalance

## 6. Kafka 驱动配置

```go
type Config struct {
	Brokers []string

	GroupID string   // 为空时用 ConsumerConfig.Group，再退回 Consumer 名

	MinBytes       int           // 默认 10e3
	MaxBytes       int           // 默认 10e6
	MaxWait        time.Duration // 默认 1s
	CommitInterval time.Duration
	QueueCapacity  int           // 默认 100
	StartOffset    int64         // 默认 kgo.LastOffset
}
```

```go
driver := mqkafka.NewWithClient(kafkaClient, cfg)   // 复用 integrations/kafka
driver := mqkafka.NewWithReaderFactory(factory, cfg) // 自定义 reader（测试友好）
```

## 7. 路由键与上下文

```go
routeKey = "mq:" + msg.Topic + ":" + cfg.Consumer
```

`HandlerContext` 中被写入：

| 位置 | 内容 |
|------|------|
| `Request.Method` | topic |
| `Request.Path` | consumer 名 |
| `Request.BodyBytes` | 原始 body |
| `Request.Headers` / `HeadersMulti` | header（单值 / 多值） |
| metadata | headers、topic、consumer、key、retryCount、ack、nack |

## 8. 内存驱动（测试）

```go
type memoryDriver struct{ ch chan *mqadapter.Message }

func (d *memoryDriver) Open(ctx context.Context, cfg mqadapter.ConsumerConfig) (mqadapter.Session, error) {
	return &memorySession{ch: d.ch}, nil
}

type memorySession struct{ ch chan *mqadapter.Message }

func (s *memorySession) Poll(ctx context.Context) (*mqadapter.Message, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-s.ch:
		return msg, nil
	}
}

func (s *memorySession) Close() error { return nil }
```

```go
driver.ch <- &mqadapter.Message{
	Topic:   "order.created",
	Headers: map[string][]string{"X-Trace-Id": {"trace-1"}},
	Body:    []byte(`{"id":"o1"}`),
	Ack:     func() error { return nil },
	Nack:    func(error) error { return nil },
}
```

推一条消息即可端到端验证 binding + AOP + runtime。
