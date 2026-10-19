---
title: MQ
weight: 5
prev: /docs/protocols/websocket
next: /docs/protocols/scheduler
---

包：`github.com/sao-lang/lania-g/protocol/mq/v3`

MQ adapter 抽象出最小的驱动边界：`Open` / `Poll` / `Close`。

## 1. 驱动接口

```go
type Driver interface {
	Open(ctx context.Context, cfg ConsumerConfig) (Session, error)
}

type Session interface {
	Poll(ctx context.Context) (*Message, error)
	Close() error
}
```

内置 Kafka 驱动（`protocol/mq/v3/kafka`），也可以自己实现内存/测试驱动。

## 2. 声明消费者

```go
root := module.New(module.Metadata{
	Imports:     []module.Module{mqadapter.Module(mqadapter.Config{Driver: driver})},
	Controllers: []any{consumer},
})

func (c *OrderConsumer) Declare(r *mqadapter.Routes) {
	r.Consumer("default").
		Group("g1").
		On("order.created", c.OnOrder).
		BindParam(3, "X-Trace-Id") // Header[T] 需要绑定名
}
```

## 3. 处理器

```go
type orderPayload struct {
	ID string `json:"id"`
}

func (c *Consumer) OnOrder(
	ctx context.Context,
	payload orderPayload,
	headers mqbinding.Headers,
	trace mqbinding.Header[string],
	topic mqbinding.Topic,
	ack mqbinding.Ack,
) error {
	if err := ack(); err != nil { // 确认消费
		return err
	}
	return nil
}
```

## 4. 绑定类型

| wrapper | 含义 |
|---------|------|
| `Message[T]` | 消息体 |
| `Header[T]` | 单个 header（需要 `BindParam`） |
| `Headers` | 全部 header |
| `Topic` / `Consumer` / `Key` | 投递元信息 |
| `RetryCount` | 重试次数 |
| `Ack` / `Nack` | 确认 / 拒绝 |

{{< callout type="info" >}}
header 名称查找是**大小写无关**的；adapter 会把 header key 归一化为小写。
{{< /callout >}}

## 5. 错误处理

- handler 返回 `error` → adapter 自动调用 `Nack(err)`
- handler 返回 nil → adapter 自动调用 `Ack()`
- `ack` / `nack` 都会被 `once` 包装，业务提前确认也不会重复提交

## 6. 测试驱动

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

推入一条消息即可验证完整链路：

```go
driver.ch <- &mqadapter.Message{
	Topic:   "order.created",
	Headers: map[string][]string{"X-Trace-Id": {"trace-1"}},
	Body:    []byte(`{"id":"o1"}`),
	Ack:     func() error { return nil },
}
```
