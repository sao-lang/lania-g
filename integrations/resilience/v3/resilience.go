// resilience.go 定义 resilience 集成的核心组合能力与对外入口。
package resilience

import (
	"context"
	"sync"
	"time"
)

const (
	// DefaultName 是默认 resilience 服务实例名。
	DefaultName                  = "default"
	// DefaultRateLimitHeader 是默认限流键请求头名。
	DefaultRateLimitHeader       = "X-RateLimit-Key"
	// DefaultIdempotencyHeader 是默认幂等键请求头名。
	DefaultIdempotencyHeader     = "Idempotency-Key"
	// MetadataKeyIdempotencyRecord 是在 runtime metadata 中保存幂等记录的键。
	MetadataKeyIdempotencyRecord = "resilience.idempotency"
)

type idempotencyContextKey string

const contextKeyIdempotencyRecord idempotencyContextKey = MetadataKeyIdempotencyRecord

// DistributedStore 定义分布式限流、幂等等策略可选依赖的共享存储能力。
type DistributedStore interface {
	Get(key string) (string, error)
	Set(key string, value string, ttl time.Duration) error
	Delete(key string) error
	Exists(key string) (bool, error)
}

// Config 描述 resilience 服务的整体配置。
type Config struct {
	Name             string
	RateLimit        RateLimitConfig
	Timeout          TimeoutConfig
	Retry            RetryConfig
	Circuit          CircuitBreakerConfig
	Idempotency      IdempotencyConfig
	Clock            func() time.Time
	DistributedStore DistributedStore
	Strategy         StrategyConfig
}

// RateLimitConfig 描述限流策略配置。
type RateLimitConfig struct {
	Enabled bool
	Limit   int
	Window  time.Duration
	Header  string
}

// TimeoutConfig 描述超时控制配置。
type TimeoutConfig struct {
	Enabled  bool
	Duration time.Duration
}

// RetryConfig 描述重试策略配置。
type RetryConfig struct {
	Enabled     bool
	MaxAttempts int
	Backoff     time.Duration
}

// CircuitBreakerConfig 描述熔断器配置。
type CircuitBreakerConfig struct {
	Enabled          bool
	FailureThreshold int
	OpenTimeout      time.Duration
}

// IdempotencyConfig 描述幂等控制配置。
type IdempotencyConfig struct {
	Enabled bool
	Header  string
	TTL     time.Duration
}

// StrategyConfig 允许按 key/场景覆盖默认 resilience 策略。
type StrategyConfig struct {
	RateLimit   map[string]RateLimitConfig
	Circuit     map[string]CircuitBreakerConfig
	Timeout     map[string]TimeoutConfig
	Retry       map[string]RetryConfig
	Idempotency map[string]IdempotencyConfig
}

// Service 是 resilience integration 对外暴露的核心服务。
type Service struct {
	config      Config
	mu          sync.Mutex
	rateWindows map[string]*rateState
	breakers    map[string]*breakerState
	idempotency map[string]*idempotentRecord
	distributed DistributedStore
}

// Factory 约定 resilience 服务工厂的最小能力。
type Factory interface {
	Default() *Service
	New(cfg Config) (*Service, error)
}

type rateState struct {
	start time.Time
	count int
}

type breakerState struct {
	failures    int
	openedUntil time.Time
}

type responseSnapshot struct {
	Status  int
	Headers map[string]string
	Body    any
}

type idempotentRecord struct {
	expiresAt time.Time
	response  responseSnapshot
}

// DefaultConfig 返回一份可用的默认 resilience 配置。
func DefaultConfig() Config {
	return Config{
		Name:        DefaultName,
		RateLimit:   RateLimitConfig{Enabled: true, Limit: 100, Window: time.Minute, Header: DefaultRateLimitHeader},
		Timeout:     TimeoutConfig{Enabled: true, Duration: 5 * time.Second},
		Retry:       RetryConfig{Enabled: true, MaxAttempts: 2, Backoff: 10 * time.Millisecond},
		Circuit:     CircuitBreakerConfig{Enabled: true, FailureThreshold: 5, OpenTimeout: 30 * time.Second},
		Idempotency: IdempotencyConfig{Enabled: true, Header: DefaultIdempotencyHeader, TTL: time.Minute},
		Clock:       time.Now,
		Strategy: StrategyConfig{
			RateLimit:   make(map[string]RateLimitConfig),
			Circuit:     make(map[string]CircuitBreakerConfig),
			Timeout:     make(map[string]TimeoutConfig),
			Retry:       make(map[string]RetryConfig),
			Idempotency: make(map[string]IdempotencyConfig),
		},
	}
}

func normalizeConfig(cfg Config) Config {
	def := DefaultConfig()
	if cfg.Name == "" {
		cfg.Name = def.Name
	}
	if cfg.RateLimit.Limit == 0 {
		cfg.RateLimit = def.RateLimit
	}
	if cfg.Timeout.Duration == 0 {
		cfg.Timeout = def.Timeout
	}
	if cfg.Retry.MaxAttempts == 0 {
		cfg.Retry = def.Retry
	}
	if cfg.Circuit.FailureThreshold == 0 {
		cfg.Circuit = def.Circuit
	}
	if cfg.Idempotency.Header == "" {
		cfg.Idempotency = def.Idempotency
	}
	if cfg.Clock == nil {
		cfg.Clock = def.Clock
	}
	if cfg.Strategy.RateLimit == nil {
		cfg.Strategy.RateLimit = def.Strategy.RateLimit
	}
	if cfg.Strategy.Circuit == nil {
		cfg.Strategy.Circuit = def.Strategy.Circuit
	}
	if cfg.Strategy.Timeout == nil {
		cfg.Strategy.Timeout = def.Strategy.Timeout
	}
	if cfg.Strategy.Retry == nil {
		cfg.Strategy.Retry = def.Strategy.Retry
	}
	if cfg.Strategy.Idempotency == nil {
		cfg.Strategy.Idempotency = def.Strategy.Idempotency
	}
	return cfg
}

// New 基于配置创建一个 resilience Service。
func New(cfg Config) (*Service, error) {
	cfg = normalizeConfig(cfg)
	return &Service{
		config:      cfg,
		rateWindows: map[string]*rateState{},
		breakers:    map[string]*breakerState{},
		idempotency: map[string]*idempotentRecord{},
		distributed: cfg.DistributedStore,
	}, nil
}

// Default 返回当前 service 本身，便于满足 Factory 风格接口。
func (s *Service) Default() *Service                { return s }
// New 以工厂风格创建新的 service。
func (s *Service) New(cfg Config) (*Service, error) { return New(cfg) }
// Config 返回当前 service 持有的配置快照。
func (s *Service) Config() Config                   { return s.config }

// ServiceToken 返回指定 resilience 实例在 DI 中使用的 token。
func ServiceToken(name string) string {
	if name == "" {
		name = DefaultName
	}
	return "resilience:instance:" + name
}

// ContextWithIdempotency 把幂等 key 写入 context，供后续链路读取。
func ContextWithIdempotency(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, contextKeyIdempotencyRecord, key)
}
