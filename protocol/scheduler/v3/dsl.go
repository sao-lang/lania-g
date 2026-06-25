// dsl.go 提供 Scheduler adapter 的声明式注册 DSL。
package scheduler

import (
	"fmt"
	"strings"
	"sync"
	"time"

	coreadapter "github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// Job 提供一个全局 DSL 兼容入口，用于声明一个定时任务：
//
//	scheduler.Job("cleanup", svc).Every(time.Minute, svc.Run).Build()
//
// 它默认通过 `NewCompatAPI()` 写入全局 registry。
// 新业务代码更推荐通过 mounted adapter 暴露的 `adapter.API()` 在应用实例上注册声明。
func Job(name string, receiver any) *JobBuilder {
	return globalCompatAPI("scheduler.Job").Job(name, receiver)
}

// API 是 scheduler adapter 对 registry 的轻量封装入口。
type API struct {
	reg            *registry.Registry
	fallbackSource string
}

// NewAPI 创建一个 scheduler DSL 入口。
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
	return globalCompatAPI("scheduler.NewCompatAPI()")
}

func globalCompatAPI(source string) *API {
	return &API{reg: registry.Global(), fallbackSource: source}
}

// Job 创建一个任务声明构建器。
func (api *API) Job(name string, receiver any) *JobBuilder {
	return newJobBuilder(name, receiver, api.reg, api.fallbackSource)
}

// JobBuilder 用于声明一个定时任务及其调度策略。
type JobBuilder struct {
	name           string
	receiver       any
	registry       *registry.Registry
	fallbackSource string
	definition     *JobDefinition
	paramBindings  map[int]string
	sealed         bool
	err            error
	mu             sync.RWMutex
}

func newJobBuilder(name string, receiver any, reg *registry.Registry, fallbackSource string) *JobBuilder {
	if reg == nil {
		reg = registry.Global()
	}
	return &JobBuilder{
		name:           name,
		receiver:       receiver,
		registry:       reg,
		fallbackSource: fallbackSource,
		paramBindings:  make(map[int]string),
	}
}

// Cron 使用 cron 表达式声明触发方式。
func (b *JobBuilder) Cron(expr string, handler any) *JobBuilder {
	return b.setTrigger(TriggerCron, expr, 0, handler)
}

// Every 使用固定时间间隔声明触发方式。
func (b *JobBuilder) Every(duration time.Duration, handler any) *JobBuilder {
	return b.setTrigger(TriggerEvery, "", duration, handler)
}

// After 使用延迟一次执行声明触发方式。
func (b *JobBuilder) After(duration time.Duration, handler any) *JobBuilder {
	return b.setTrigger(TriggerAfter, "", duration, handler)
}

// BindParam 为某个参数索引绑定一个名字，供编译期/运行期参数解析使用。
func (b *JobBuilder) BindParam(paramIndex int, name string) *JobBuilder {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sealed || name == "" {
		return b
	}
	b.paramBindings[paramIndex] = name
	return b
}

// Retry 设置失败后的重试次数与退避时间。
func (b *JobBuilder) Retry(attempts int, backoff time.Duration) *JobBuilder {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sealed || b.definition == nil {
		return b
	}
	if attempts > 0 {
		b.definition.RetryAttempts = attempts
	}
	if backoff > 0 {
		b.definition.RetryBackoff = backoff
	}
	return b
}

// MaxConcurrency 设置该任务允许的最大并发执行数。
func (b *JobBuilder) MaxConcurrency(n int) *JobBuilder {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sealed || b.definition == nil {
		return b
	}
	if n > 0 {
		b.definition.MaxConcurrency = n
	}
	return b
}

// Unique 打开唯一执行约束，并可选指定唯一键。
func (b *JobBuilder) Unique(key ...string) *JobBuilder {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sealed || b.definition == nil {
		return b
	}
	b.definition.Unique = true
	if len(key) > 0 && key[0] != "" {
		b.definition.UniqueKey = key[0]
	}
	return b
}

// WithTimeout 设置单次任务执行的超时时间。
func (b *JobBuilder) WithTimeout(timeout time.Duration) *JobBuilder {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sealed || b.definition == nil {
		return b
	}
	if timeout > 0 {
		b.definition.Timeout = timeout
	}
	return b
}

// Misfire 设置错过调度时的处理策略。
func (b *JobBuilder) Misfire(policy MisfirePolicy) *JobBuilder {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sealed || b.definition == nil {
		return b
	}
	if policy != "" {
		b.definition.MisfirePolicy = policy
	}
	return b
}

// Build 完成当前任务声明并写入 registry；忽略构建错误。
func (b *JobBuilder) Build() *JobDefinition {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sealed = true
	return b.buildAndRegisterLocked()
}

// BuildE 完成当前任务声明并写入 registry；如果声明不合法则返回错误。
func (b *JobBuilder) BuildE() (*JobDefinition, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sealed = true
	if err := b.validateLocked(); err != nil {
		b.err = err
		return nil, err
	}
	return b.buildAndRegisterLocked(), nil
}

func (b *JobBuilder) buildAndRegisterLocked() *JobDefinition {
	if b.definition == nil {
		return nil
	}
	b.definition.ParamBindings = coreadapter.CopyIntStringMap(b.paramBindings)
	if b.fallbackSource != "" {
		b.registry.RecordFallbackUsage(b.fallbackSource)
	}
	b.registry.RegisterDecl(AdapterID, "jobs", b.definition)
	return b.definition
}

// Err 返回 DSL 构建过程中记录下来的错误。
func (b *JobBuilder) Err() error { return b.err }

func (b *JobBuilder) setTrigger(kind TriggerKind, expr string, duration time.Duration, handler any) *JobBuilder {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sealed {
		return b
	}
	b.definition = &JobDefinition{
		Name:           b.name,
		TriggerKind:    kind,
		Expression:     expr,
		Duration:       duration,
		Receiver:       b.receiver,
		HandlerName:    coreadapter.FindMethodName(b.receiver, handler),
		MaxConcurrency: 1,
		MisfirePolicy:  MisfireQueue,
		ParamBindings:  coreadapter.CopyIntStringMap(b.paramBindings),
	}
	return b
}

func (b *JobBuilder) validateLocked() error {
	if strings.TrimSpace(b.name) == "" {
		return fmt.Errorf("scheduler job name is required")
	}
	if b.receiver == nil {
		return fmt.Errorf("scheduler job receiver is nil")
	}
	if b.definition == nil {
		return fmt.Errorf("scheduler job %s has no trigger definition", b.name)
	}
	if strings.TrimSpace(b.definition.HandlerName) == "" {
		return fmt.Errorf("invalid scheduler job declaration: %s", b.name)
	}
	switch b.definition.TriggerKind {
	case TriggerCron:
		if strings.TrimSpace(b.definition.Expression) == "" {
			return fmt.Errorf("scheduler cron expression is required for job %s", b.name)
		}
	case TriggerEvery, TriggerAfter:
		if b.definition.Duration <= 0 {
			return fmt.Errorf("scheduler duration must be positive for job %s", b.name)
		}
	}
	return nil
}
