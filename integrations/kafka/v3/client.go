// client.go 实现 kafka integration 的客户端封装与基础连接/调用能力。
package kafka

import (
	"fmt"
	"time"

	kgo "github.com/segmentio/kafka-go"
)

// Config 描述 Kafka client 的基础连接与 writer 默认配置。
type Config struct {
	Name    string
	Brokers []string
	Dialer  *kgo.Dialer

	WriterBalancer     kgo.Balancer
	WriterMaxAttempts  int
	WriterBatchSize    int
	WriterBatchBytes   int
	WriterBatchTimeout time.Duration
	WriterReadTimeout  time.Duration
	WriterWriteTimeout time.Duration
	WriterRequiredAcks int
	WriterAsync        bool
}

// ReaderFactory 定义创建 `kafka-go.Reader` 的工厂接口。
type ReaderFactory interface {
	NewReader(cfg kgo.ReaderConfig) *kgo.Reader
}

// WriterFactory 定义创建 `kafka-go.Writer` 的工厂接口。
type WriterFactory interface {
	NewWriter(cfg kgo.WriterConfig) *kgo.Writer
}

// Factory 定义 Kafka Client 的通用工厂接口。
type Factory interface {
	Default() *Client
	New(cfg Config) (*Client, error)
	ReaderFactory
	WriterFactory
}

// Client 是对 `segmentio/kafka-go` 的轻量封装。
type Client struct {
	cfg Config
}

// NewClient 基于配置创建一个 Kafka Client。
func NewClient(cfg Config) (*Client, error) {
	cfg = normalizeConfig(cfg)
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafka client requires brokers")
	}
	return &Client{cfg: cfg}, nil
}

// Config 返回当前 Client 的配置副本。
func (c *Client) Config() Config { return c.cfg.clone() }

// Default 返回当前 Client 本身，用于满足 Factory 接口。
func (c *Client) Default() *Client { return c }

// New 基于 cfg 创建一个新的 Client，用于满足 Factory 接口。
func (c *Client) New(cfg Config) (*Client, error) { return NewClient(cfg) }

// Brokers 返回 broker 列表副本。
func (c *Client) Brokers() []string {
	return append([]string{}, c.cfg.Brokers...)
}

// NewReader 创建一个 reader；若 cfg 未显式提供 broker/dialer，则使用 Client 默认值。
func (c *Client) NewReader(cfg kgo.ReaderConfig) *kgo.Reader {
	if len(cfg.Brokers) == 0 {
		cfg.Brokers = append([]string{}, c.cfg.Brokers...)
	}
	if cfg.Dialer == nil {
		cfg.Dialer = c.cfg.Dialer
	}
	return kgo.NewReader(cfg)
}

// NewWriter 创建一个 writer；若 cfg 未显式提供字段，则尽量回退到 Client 默认值。
func (c *Client) NewWriter(cfg kgo.WriterConfig) *kgo.Writer {
	if len(cfg.Brokers) == 0 {
		cfg.Brokers = append([]string{}, c.cfg.Brokers...)
	}
	if cfg.Dialer == nil {
		cfg.Dialer = c.cfg.Dialer
	}
	if cfg.Balancer == nil {
		cfg.Balancer = c.cfg.WriterBalancer
	}
	if cfg.MaxAttempts == 0 && c.cfg.WriterMaxAttempts > 0 {
		cfg.MaxAttempts = c.cfg.WriterMaxAttempts
	}
	if cfg.BatchSize == 0 && c.cfg.WriterBatchSize > 0 {
		cfg.BatchSize = c.cfg.WriterBatchSize
	}
	if cfg.BatchBytes == 0 && c.cfg.WriterBatchBytes > 0 {
		cfg.BatchBytes = c.cfg.WriterBatchBytes
	}
	if cfg.BatchTimeout == 0 && c.cfg.WriterBatchTimeout > 0 {
		cfg.BatchTimeout = c.cfg.WriterBatchTimeout
	}
	if cfg.ReadTimeout == 0 && c.cfg.WriterReadTimeout > 0 {
		cfg.ReadTimeout = c.cfg.WriterReadTimeout
	}
	if cfg.WriteTimeout == 0 && c.cfg.WriterWriteTimeout > 0 {
		cfg.WriteTimeout = c.cfg.WriterWriteTimeout
	}
	if cfg.RequiredAcks == 0 && c.cfg.WriterRequiredAcks != 0 {
		cfg.RequiredAcks = c.cfg.WriterRequiredAcks
	}
	if !cfg.Async && c.cfg.WriterAsync {
		cfg.Async = true
	}
	return kgo.NewWriter(cfg)
}

func normalizeConfig(cfg Config) Config {
	if cfg.Name == "" {
		cfg.Name = "default"
	}
	if cfg.WriterBalancer == nil {
		cfg.WriterBalancer = &kgo.LeastBytes{}
	}
	if cfg.WriterMaxAttempts <= 0 {
		cfg.WriterMaxAttempts = 10
	}
	if cfg.WriterBatchSize <= 0 {
		cfg.WriterBatchSize = 100
	}
	if cfg.WriterBatchBytes <= 0 {
		cfg.WriterBatchBytes = 1048576
	}
	if cfg.WriterBatchTimeout <= 0 {
		cfg.WriterBatchTimeout = time.Second
	}
	if cfg.WriterReadTimeout <= 0 {
		cfg.WriterReadTimeout = 10 * time.Second
	}
	if cfg.WriterWriteTimeout <= 0 {
		cfg.WriterWriteTimeout = 10 * time.Second
	}
	if cfg.WriterRequiredAcks == 0 {
		cfg.WriterRequiredAcks = int(kgo.RequireAll)
	}
	return cfg
}

func (c Config) clone() Config {
	out := c
	out.Brokers = append([]string{}, c.Brokers...)
	return out
}
