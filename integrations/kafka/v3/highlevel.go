// highlevel.go 实现 kafka 集成面向高级用法的封装与辅助能力。
package kafka

import (
	stdctx "context"
	"encoding/json"
	"time"

	kgo "github.com/segmentio/kafka-go"
	mqadapter "github.com/sao-lang/lania-g/protocol/mq/v3"
)

// Message 复用 `kafka-go` 的消息结构。
type Message = kgo.Message

// MessageHandler 定义消费单条消息时的处理函数。
type MessageHandler func(stdctx.Context, Message) error

// ReaderClient 抽象 Kafka reader 的最小能力集合。
type ReaderClient interface {
	FetchMessage(stdctx.Context) (kgo.Message, error)
	CommitMessages(stdctx.Context, ...kgo.Message) error
	Close() error
}

// WriterClient 抽象 Kafka writer 的最小能力集合。
type WriterClient interface {
	WriteMessages(stdctx.Context, ...kgo.Message) error
	Close() error
}

// ProducerConfig 描述 Producer 的主题、重试与 writer 配置。
type ProducerConfig struct {
	Topic         string
	DLQTopic      string
	RetryAttempts int
	RetryBackoff  time.Duration
	WriterConfig  kgo.WriterConfig
}

// PublishOptions 描述一次 PublishValue 调用的覆盖选项。
type PublishOptions struct {
	Topic         string
	Key           string
	Headers       map[string]string
	RetryAttempts int
	RetryBackoff  time.Duration
	DLQTopic      string
}

// ConsumerConfig 描述 Consumer 的主题、消费组、重试与 reader 配置。
type ConsumerConfig struct {
	Topic         string
	GroupID       string
	DLQTopic      string
	RetryAttempts int
	RetryBackoff  time.Duration
	BatchSize     int
	AutoCommit    bool
	ReaderConfig  kgo.ReaderConfig
}

// ConsumeOptions 描述从 MQ 配置转换时的附加消费选项。
type ConsumeOptions struct {
	GroupID       string
	Topic         string
	BatchSize     int
	AutoCommit    bool
	RetryAttempts int
	RetryBackoff  time.Duration
	DLQTopic      string
}

// Producer 是面向业务的高级生产者封装。
type Producer struct {
	cfg       ProducerConfig
	writer    WriterClient
	dlqWriter WriterClient
}

// Consumer 是面向业务的高级消费者封装。
type Consumer struct {
	cfg       ConsumerConfig
	reader    ReaderClient
	dlqWriter WriterClient
}

// NewProducer 基于 Client 默认配置创建一个高级 Producer。
func (c *Client) NewProducer(cfg ProducerConfig) *Producer {
	if cfg.Topic != "" && cfg.WriterConfig.Topic == "" {
		cfg.WriterConfig.Topic = cfg.Topic
	}
	if cfg.RetryAttempts <= 0 {
		cfg.RetryAttempts = 1
	}
	writer := c.NewWriter(cfg.WriterConfig)
	var dlqWriter WriterClient
	if cfg.DLQTopic != "" {
		dlqCfg := cfg.WriterConfig
		dlqCfg.Topic = cfg.DLQTopic
		dlqWriter = c.NewWriter(dlqCfg)
	}
	return &Producer{cfg: cfg, writer: writer, dlqWriter: dlqWriter}
}

// NewProducerWithWriter 基于自定义 writer 创建 Producer，适合测试或特殊注入场景。
func NewProducerWithWriter(cfg ProducerConfig, writer WriterClient, dlqWriter WriterClient) *Producer {
	if cfg.RetryAttempts <= 0 {
		cfg.RetryAttempts = 1
	}
	return &Producer{cfg: cfg, writer: writer, dlqWriter: dlqWriter}
}

// Publish 发布单条消息。
func (p *Producer) Publish(ctx stdctx.Context, message Message) error {
	return p.PublishBatch(ctx, []Message{message})
}

// PublishValue 先把 value 序列化为 JSON，再按 opts 构造消息并发布。
func (p *Producer) PublishValue(ctx stdctx.Context, value interface{}, opts PublishOptions) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	msg := kgo.Message{Topic: opts.Topic, Key: []byte(opts.Key), Value: payload}
	for key, value := range opts.Headers {
		msg.Headers = append(msg.Headers, kgo.Header{Key: key, Value: []byte(value)})
	}
	cfg := p.cfg
	if opts.RetryAttempts > 0 {
		cfg.RetryAttempts = opts.RetryAttempts
	}
	if opts.RetryBackoff > 0 {
		cfg.RetryBackoff = opts.RetryBackoff
	}
	if opts.DLQTopic != "" {
		cfg.DLQTopic = opts.DLQTopic
	}
	if opts.Topic != "" {
		msg.Topic = opts.Topic
	}
	clone := &Producer{cfg: cfg, writer: p.writer, dlqWriter: p.dlqWriter}
	return clone.Publish(ctx, msg)
}

// PublishBatch 批量发布消息；失败时会按配置重试，并可写入 DLQ。
func (p *Producer) PublishBatch(ctx stdctx.Context, messages []Message) error {
	if len(messages) == 0 {
		return nil
	}
	var lastErr error
	for attempt := 0; attempt < p.cfg.RetryAttempts; attempt++ {
		if err := p.writer.WriteMessages(ctx, messages...); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt+1 < p.cfg.RetryAttempts && p.cfg.RetryBackoff > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(p.cfg.RetryBackoff):
			}
		}
	}
	if lastErr != nil && p.dlqWriter != nil {
		_ = p.dlqWriter.WriteMessages(ctx, withDLQHeaders(messages, lastErr.Error())...)
	}
	return lastErr
}

// Close 关闭 Producer 持有的 writer。
func (p *Producer) Close() error {
	if p.writer != nil {
		_ = p.writer.Close()
	}
	if p.dlqWriter != nil {
		_ = p.dlqWriter.Close()
	}
	return nil
}

// NewConsumer 基于 Client 默认配置创建一个高级 Consumer。
func (c *Client) NewConsumer(cfg ConsumerConfig) *Consumer {
	if cfg.Topic != "" && cfg.ReaderConfig.Topic == "" {
		cfg.ReaderConfig.Topic = cfg.Topic
	}
	if cfg.GroupID != "" && cfg.ReaderConfig.GroupID == "" {
		cfg.ReaderConfig.GroupID = cfg.GroupID
	}
	if cfg.RetryAttempts <= 0 {
		cfg.RetryAttempts = 1
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 1
	}
	reader := c.NewReader(cfg.ReaderConfig)
	var dlqWriter WriterClient
	if cfg.DLQTopic != "" {
		dlqWriter = c.NewWriter(kgo.WriterConfig{Topic: cfg.DLQTopic})
	}
	return &Consumer{cfg: cfg, reader: reader, dlqWriter: dlqWriter}
}

// NewConsumerWithReader 基于自定义 reader 创建 Consumer，适合测试或特殊注入场景。
func NewConsumerWithReader(cfg ConsumerConfig, reader ReaderClient, dlqWriter WriterClient) *Consumer {
	if cfg.RetryAttempts <= 0 {
		cfg.RetryAttempts = 1
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 1
	}
	return &Consumer{cfg: cfg, reader: reader, dlqWriter: dlqWriter}
}

// ConsumeOnce 消费一条消息并交给 handler 处理。
func (c *Consumer) ConsumeOnce(ctx stdctx.Context, handler MessageHandler) error {
	msg, err := c.reader.FetchMessage(ctx)
	if err != nil {
		return err
	}
	if err := c.process(ctx, msg, handler); err != nil {
		return err
	}
	if c.cfg.AutoCommit {
		return c.reader.CommitMessages(ctx, msg)
	}
	return nil
}

// ConsumeBatch 按配置批量拉取并处理消息。
func (c *Consumer) ConsumeBatch(ctx stdctx.Context, handler MessageHandler) error {
	messages := make([]kgo.Message, 0, c.cfg.BatchSize)
	for i := 0; i < c.cfg.BatchSize; i++ {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if len(messages) == 0 {
				return err
			}
			break
		}
		messages = append(messages, msg)
	}
	for _, msg := range messages {
		if err := c.process(ctx, msg, handler); err != nil {
			return err
		}
	}
	if c.cfg.AutoCommit && len(messages) > 0 {
		return c.reader.CommitMessages(ctx, messages...)
	}
	return nil
}

// Run 持续循环消费，直到 ctx 结束或发生错误。
func (c *Consumer) Run(ctx stdctx.Context, handler MessageHandler) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := c.ConsumeBatch(ctx, handler); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
	}
}

// ConsumerConfigFromMQ 把 MQ adapter 的消费配置映射为 Kafka ConsumerConfig。
func ConsumerConfigFromMQ(cfg mqadapter.ConsumerConfig, topic string, opts ConsumeOptions) ConsumerConfig {
	out := ConsumerConfig{
		Topic:         topic,
		GroupID:       firstNonEmpty(opts.GroupID, cfg.Group),
		DLQTopic:      opts.DLQTopic,
		RetryAttempts: opts.RetryAttempts,
		RetryBackoff:  opts.RetryBackoff,
		BatchSize:     opts.BatchSize,
		AutoCommit:    opts.AutoCommit,
	}
	if out.BatchSize <= 0 {
		out.BatchSize = 1
	}
	return out
}

func (c *Consumer) process(ctx stdctx.Context, msg kgo.Message, handler MessageHandler) error {
	var lastErr error
	for attempt := 0; attempt < c.cfg.RetryAttempts; attempt++ {
		if err := handler(ctx, msg); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt+1 < c.cfg.RetryAttempts && c.cfg.RetryBackoff > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.cfg.RetryBackoff):
			}
		}
	}
	if lastErr != nil && c.dlqWriter != nil {
		_ = c.dlqWriter.WriteMessages(ctx, withDLQHeaders([]kgo.Message{msg}, lastErr.Error())...)
	}
	return lastErr
}

// Close 关闭 Consumer 持有的 reader 与 DLQ writer。
func (c *Consumer) Close() error {
	if c.reader != nil {
		_ = c.reader.Close()
	}
	if c.dlqWriter != nil {
		_ = c.dlqWriter.Close()
	}
	return nil
}

func withDLQHeaders(messages []kgo.Message, reason string) []kgo.Message {
	out := make([]kgo.Message, 0, len(messages))
	for _, msg := range messages {
		clone := msg
		clone.Headers = append(append([]kgo.Header{}, msg.Headers...), kgo.Header{
			Key:   "x-dlq-reason",
			Value: []byte(reason),
		})
		out = append(out, clone)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
