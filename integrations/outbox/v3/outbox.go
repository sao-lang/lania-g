// outbox.go 定义 outbox 模式的核心对象与基础能力。
package outbox

import (
	"context"
	"sync"
	"time"

	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// DefaultName 是默认 outbox 服务实例名。
const DefaultName = "default"

// Status 表示 outbox 消息的当前状态。
type Status string

const (
	// StatusPending 表示消息已入库但尚未派发。
	StatusPending    Status = "pending"
	// StatusDispatched 表示消息已成功派发。
	StatusDispatched Status = "dispatched"
	// StatusFailed 表示消息派发失败但仍可重试。
	StatusFailed     Status = "failed"
	// StatusDead 表示消息已进入死信状态，不再自动重试。
	StatusDead       Status = "dead"
)

// SchedulerConfig 描述 outbox 调度器的轮询和并发配置。
type SchedulerConfig struct {
	Enabled     bool
	Interval    time.Duration
	BatchSize   int
	Concurrency int
}

// Config 描述 outbox 服务的初始化配置。
type Config struct {
	Name        string
	MaxAttempts int
	Store       Store
	Dispatcher  Dispatcher
	DeadLetter  Dispatcher
	Scheduler   SchedulerConfig
}

// Message 表示一条待投递或已投递的 outbox 消息记录。
type Message struct {
	ID          string
	Topic       string
	Key         string
	Payload     []byte
	Headers     map[string]string
	Status      Status
	Attempts    int
	LastError   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	AvailableAt time.Time
}

// Store 定义 outbox 持久化存储需要实现的最小能力。
type Store interface {
	Save(context.Context, *Message) error
	Get(context.Context, string) (*Message, error)
	ListPending(context.Context, int) ([]*Message, error)
	Mark(context.Context, string, Status, string, int) error
}

// Dispatcher 负责把一条消息真正投递到外部系统。
type Dispatcher interface {
	Dispatch(context.Context, *Message) error
}

// DispatcherFunc 让普通函数适配 Dispatcher 风格调用。
type DispatcherFunc func(context.Context, *Message) error

// Service 是 outbox 模块对外暴露的核心服务。
type Service struct {
	config     Config
	store      Store
	dispatcher Dispatcher
	deadLetter Dispatcher
	inbox      *Inbox
	scheduler  *Scheduler
}

// Scheduler 负责后台轮询并批量派发待处理消息。
type Scheduler struct {
	service *Service
	config  SchedulerConfig
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// Factory 约定 outbox 服务工厂的最小能力。
type Factory interface {
	Default() *Service
	New(cfg Config) (*Service, error)
}

// Module 是 outbox integration 对应的模块封装。
type Module struct {
	*module.BaseModule
	service      *Service
	config       Config
	reg          *registry.Registry
	compatSource string
}

// Inbox 用于做简单的消息去重控制。
type Inbox struct {
	mu        sync.Mutex
	processed map[string]time.Time
}

// InjectService 用于在 DI 中以更明确的语义注入 `*Service`。
type InjectService struct{ *Service }

// DefaultConfig 返回一份可用的默认 outbox 配置。
func DefaultConfig() Config {
	return Config{
		Name:        DefaultName,
		MaxAttempts: 3,
		Scheduler: SchedulerConfig{
			Enabled:     true,
			Interval:    5 * time.Second,
			BatchSize:   100,
			Concurrency: 5,
		},
	}
}

func normalizeConfig(cfg Config) Config {
	def := DefaultConfig()
	if cfg.Name == "" {
		cfg.Name = def.Name
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = def.MaxAttempts
	}
	if cfg.Store == nil {
		cfg.Store = NewMemoryStore()
	}
	if !cfg.Scheduler.Enabled {
		cfg.Scheduler = def.Scheduler
	} else {
		if cfg.Scheduler.Interval <= 0 {
			cfg.Scheduler.Interval = def.Scheduler.Interval
		}
		if cfg.Scheduler.BatchSize <= 0 {
			cfg.Scheduler.BatchSize = def.Scheduler.BatchSize
		}
		if cfg.Scheduler.Concurrency <= 0 {
			cfg.Scheduler.Concurrency = def.Scheduler.Concurrency
		}
	}
	return cfg
}

// New 基于配置创建一个 outbox Service。
func New(cfg Config) (*Service, error) {
	cfg = normalizeConfig(cfg)
	service := &Service{
		config:     cfg,
		store:      cfg.Store,
		dispatcher: cfg.Dispatcher,
		deadLetter: cfg.DeadLetter,
		inbox:      NewInbox(),
	}

	if cfg.Scheduler.Enabled {
		service.scheduler = NewScheduler(service, cfg.Scheduler)
	}

	return service, nil
}

// Default 返回当前 service 本身，便于满足 Factory 风格接口。
func (s *Service) Default() *Service { return s }

// New 以工厂风格创建新的 service。
func (s *Service) New(cfg Config) (*Service, error) { return New(cfg) }

// Config 返回当前 service 持有的配置快照。
func (s *Service) Config() Config { return s.config }

// NewInbox 创建一个用于幂等去重的 Inbox。
func NewInbox() *Inbox {
	return &Inbox{processed: map[string]time.Time{}}
}
