// driver.go 实现基于 Kafka 的 MQ 驱动适配。
package kafka

import (
	stdctx "context"
	"fmt"
	"sync"
	"time"

	mqadapter "github.com/sao-lang/lania-g/protocol/mq/v3"
	kafkaintegration "github.com/sao-lang/lania-g/integrations/kafka/v3"

	kgo "github.com/segmentio/kafka-go"
)

// Config 描述 Kafka MQ driver 的连接与 reader 行为配置。
// 这些参数基本直接映射到 `segmentio/kafka-go` 的 ReaderConfig。
type Config struct {
	Brokers []string

	// GroupID 是可选的默认 consumer group id：
	// 当为空时优先使用 mq.ConsumerConfig.Group；
	// 若仍为空则使用 mq.ConsumerConfig.Consumer。
	GroupID string

	MinBytes       int
	MaxBytes       int
	MaxWait        time.Duration
	CommitInterval time.Duration
	QueueCapacity  int
	StartOffset    int64
}

// Driver 是 MQ adapter 的 Kafka 驱动实现，用于为每个 consumer 打开一个 session。
// 它只负责 transport 层的拉取与提交，不关心上层 runtime/binding/DSL 细节。
type Driver struct {
	cfg Config

	newReader func(kgo.ReaderConfig) reader
}

// New 创建一个 Kafka driver。
func New(cfg Config) *Driver {
	cfg = normalizeConfig(cfg)
	return NewWithReaderFactory(nil, cfg)
}

// NewWithClient 基于 integrations/kafka 的 client 创建一个 Kafka driver。
func NewWithClient(client *kafkaintegration.Client, cfg Config) *Driver {
	return NewWithReaderFactory(client, cfg)
}

// NewWithReaderFactory 使用自定义 ReaderFactory 创建一个 Kafka driver。
func NewWithReaderFactory(factory kafkaintegration.ReaderFactory, cfg Config) *Driver {
	cfg = normalizeConfig(cfg)
	d := &Driver{cfg: cfg}
	if factory != nil {
		d.newReader = func(rc kgo.ReaderConfig) reader {
			return factory.NewReader(rc)
		}
	} else {
		d.newReader = func(rc kgo.ReaderConfig) reader {
			return kgo.NewReader(rc)
		}
	}
	return d
}

// Open 根据 consumer 配置创建一个会话，并返回可轮询的 mqadapter.Session。
// 单 topic 走 `Topic`，多 topic 走 `GroupTopics`，其余 reader 参数来自 driver 级默认配置。
func (d *Driver) Open(ctx stdctx.Context, cfg mqadapter.ConsumerConfig) (mqadapter.Session, error) {
	if len(d.cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka driver requires brokers")
	}
	if len(cfg.Topics) == 0 {
		return nil, fmt.Errorf("kafka driver requires at least one topic for consumer %s", cfg.Consumer)
	}

	groupID := cfg.Group
	if groupID == "" {
		groupID = d.cfg.GroupID
	}
	if groupID == "" {
		groupID = cfg.Consumer
	}

	rc := kgo.ReaderConfig{
		Brokers:        append([]string{}, d.cfg.Brokers...),
		GroupID:        groupID,
		MinBytes:       d.cfg.MinBytes,
		MaxBytes:       d.cfg.MaxBytes,
		MaxWait:        d.cfg.MaxWait,
		CommitInterval: d.cfg.CommitInterval,
		QueueCapacity:  d.cfg.QueueCapacity,
		StartOffset:    d.cfg.StartOffset,
	}
	if len(cfg.Topics) == 1 {
		rc.Topic = cfg.Topics[0]
	} else {
		rc.GroupTopics = append([]string{}, cfg.Topics...)
	}

	return &session{
		reader: d.newReader(rc),
	}, nil
}

// normalizeConfig 为未显式提供的 Kafka reader 参数补默认值。
func normalizeConfig(cfg Config) Config {
	if cfg.MinBytes <= 0 {
		cfg.MinBytes = 10e3
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = 10e6
	}
	if cfg.MaxWait <= 0 {
		cfg.MaxWait = time.Second
	}
	if cfg.QueueCapacity <= 0 {
		cfg.QueueCapacity = 100
	}
	if cfg.StartOffset == 0 {
		cfg.StartOffset = kgo.LastOffset
	}
	return cfg
}

// reader 抽出最小依赖接口，便于测试时替换底层 kafka-go Reader。
type reader interface {
	FetchMessage(ctx stdctx.Context) (kgo.Message, error)
	CommitMessages(ctx stdctx.Context, msgs ...kgo.Message) error
	Close() error
}

type session struct {
	reader reader
}

// Poll 从 Kafka 拉取一条消息，并把其转换为 mqadapter.Message。
// 这里不直接提交 offset；提交动作被包装成 Ack，交由上层在 handler 成功后再决定是否调用。
func (s *session) Poll(ctx stdctx.Context) (*mqadapter.Message, error) {
	msg, err := s.reader.FetchMessage(ctx)
	if err != nil {
		return nil, err
	}

	headers := make(map[string][]string, len(msg.Headers))
	for _, h := range msg.Headers {
		headers[h.Key] = append(headers[h.Key], string(h.Value))
	}

	ack := commitOnce(s.reader, msg)
	return &mqadapter.Message{
		Topic:      msg.Topic,
		Key:        string(msg.Key),
		Headers:    headers,
		Body:       append([]byte{}, msg.Value...),
		Raw:        msg,
		RetryCount: 0,
		Ack:        ack,
		Nack: func(error) error {
			// group consumer 的 nack 策略目前很保守：不提交 offset。
			// 是否重投以及何时重投，交给 broker/group rebalance 语义处理。
			return nil
		},
	}, nil
}

// Close 关闭底层 reader 并释放资源。
func (s *session) Close() error {
	if s.reader == nil {
		return nil
	}
	return s.reader.Close()
}

// commitOnce 确保同一条 Kafka 消息的 offset 最多只提交一次。
func commitOnce(r reader, msg kgo.Message) func() error {
	var once sync.Once
	var err error
	return func() error {
		once.Do(func() {
			err = r.CommitMessages(stdctx.Background(), msg)
		})
		return err
	}
}

var _ mqadapter.Driver = (*Driver)(nil)
