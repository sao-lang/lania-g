// scheduler.go 实现 Scheduler adapter 的主入口与宿主集成逻辑。
package scheduler

import (
	stdctx "context"
	"fmt"
	"sync"
	"time"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
)

// Adapter 是 Scheduler 协议的运行期适配器实现。
type Adapter struct {
	host adapter.Host
	api  *API

	mu           sync.Mutex
	cancel       stdctx.CancelFunc
	started      bool
	limiters     map[string]chan struct{}
	uniqueLocker UniqueLocker
	historyLimit int
	definitions  map[string]*JobDefinition
	metrics      map[string]*JobMetrics
	history      map[string][]JobRunRecord
}

// New 创建 Scheduler adapter。
//
// 它负责把调度声明转换为后台执行中的 job runner，
// 并维护限流、唯一执行、运行历史和指标等调度期状态。
func New() *Adapter {
	return &Adapter{
		api:          NewCompatAPI(),
		limiters:     make(map[string]chan struct{}),
		uniqueLocker: NewInMemoryUniqueLocker(),
		historyLimit: 20,
		definitions:  make(map[string]*JobDefinition),
		metrics:      make(map[string]*JobMetrics),
		history:      make(map[string][]JobRunRecord),
	}
}

// ID 返回该 adapter 的唯一标识。
func (a *Adapter) ID() string { return AdapterID }

// Plugins 返回 Scheduler 协议参与编译的插件列表。
func (a *Adapter) Plugins() []compiler.ProtocolPlugin { return []compiler.ProtocolPlugin{NewPlugin()} }

// API 返回 Scheduler adapter 暴露给应用侧的 DSL API。
func (a *Adapter) API() any { return a.api }

// Mount 将 Scheduler adapter 挂到应用 host 上，并把 API 绑定到当前 registry。
func (a *Adapter) Mount(host adapter.Host) error {
	if host == nil {
		return fmt.Errorf("scheduler adapter host is nil")
	}
	a.host = host
	a.api = NewAPI(host.Registry())
	return nil
}

// Start 启动 Scheduler adapter。
//
// 它会从 registry 中收集所有 job 定义，然后根据触发器类型分别启动
// cron/ticker/timer 等后台执行循环。
func (a *Adapter) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.started {
		return nil
	}
	if a.host == nil {
		return fmt.Errorf("scheduler adapter not mounted")
	}
	defs := collectJobs(a.host.Registry())
	if len(defs) == 0 {
		a.started = true
		return nil
	}
	ctx, cancel := stdctx.WithCancel(stdctx.Background())
	a.cancel = cancel
	for _, def := range defs {
		if err := a.prepareAndStartJob(ctx, def); err != nil {
			cancel()
			return err
		}
	}
	a.started = true
	return nil
}

// prepareAndStartJob 把一个 JobDefinition 转成实际运行中的后台调度器。
// 这里会先初始化 definition/metrics/limiter 等运行期状态，再按 TriggerKind 选择具体驱动循环。
func (a *Adapter) prepareAndStartJob(ctx stdctx.Context, def *JobDefinition) error {
	key := a.limiterKey(def)
	a.definitions[key] = cloneJobDefinition(def)
	a.ensureMetrics(def)
	a.updateNextRunLocked(def, time.Now())
	a.ensureLimiter(def)
	switch def.TriggerKind {
	case TriggerCron:
		if err := validateCronExpression(def.Expression); err != nil {
			return err
		}
		go a.runCron(ctx, def)
	case TriggerEvery:
		go a.runTicker(ctx, def)
	case TriggerAfter:
		go a.runTimer(ctx, def)
	default:
		return fmt.Errorf("unsupported scheduler trigger kind: %s", def.TriggerKind)
	}
	return nil
}

// Stop 停止 Scheduler adapter，并清空运行期状态。
// 这里不仅停止后台 goroutine，也会把 metrics/history/definitions 等内存态一起清空。
func (a *Adapter) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.started = false
	a.limiters = make(map[string]chan struct{})
	a.definitions = make(map[string]*JobDefinition)
	a.metrics = make(map[string]*JobMetrics)
	a.history = make(map[string][]JobRunRecord)
	return nil
}

// SetUniqueLocker 设置唯一执行锁实现。
// 替换后只影响后续触发，不会追溯修改已经在运行中的 job。
func (a *Adapter) SetUniqueLocker(locker UniqueLocker) {
	if locker == nil {
		return
	}
	a.mu.Lock()
	a.uniqueLocker = locker
	a.mu.Unlock()
}

// SetHistoryLimit 设置每个 job 记录的历史条数上限。
func (a *Adapter) SetHistoryLimit(limit int) {
	if limit <= 0 {
		return
	}
	a.mu.Lock()
	a.historyLimit = limit
	a.mu.Unlock()
}

var _ adapter.Adapter = (*Adapter)(nil)
