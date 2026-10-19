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

// Controller 由需要声明定时任务的控制器实现。
type Controller interface {
	Declare(*Routes)
}

// Routes 收集一个控制器声明的全部定时任务。
//
// 它是 scheduler 唯一的声明入口：控制器在所属模块内完成注册，无需手动 Build。
type Routes struct {
	reg      *registry.Registry
	receiver any
	current  *JobBuilder
	err      error
}

func newRoutes(reg *registry.Registry, receiver any) *Routes {
	return &Routes{reg: reg, receiver: receiver}
}

// Job 开始声明一个定时任务。
func (r *Routes) Job(name string) *JobBuilder {
	if err := r.sealCurrent(); err != nil && r.err == nil {
		r.err = err
	}
	b := newJobBuilder(name, r.receiver, r)
	r.current = b
	return b
}

// Err 结束声明并返回累计错误。
func (r *Routes) Err() error {
	if r == nil {
		return nil
	}
	if err := r.sealCurrent(); err != nil && r.err == nil {
		r.err = err
	}
	return r.err
}

func (r *Routes) sealCurrent() error {
	if r == nil || r.current == nil {
		return nil
	}
	b := r.current
	r.current = nil
	return b.seal()
}

// JobBuilder 用于声明一个定时任务及其调度策略。
type JobBuilder struct {
	name           string
	receiver       any
	routes         *Routes
	definition     *JobDefinition
	paramBindings  map[int]string
	sealed         bool
	err            error
	mu             sync.RWMutex
}

func newJobBuilder(name string, receiver any, routes *Routes) *JobBuilder {
	return &JobBuilder{
		name:          name,
		receiver:      receiver,
		routes:        routes,
		paramBindings: make(map[int]string),
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
// BuildE 完成当前任务声明并写入 registry；如果声明不合法则返回错误。
// seal 完成当前任务声明并写入 registry。
func (b *JobBuilder) seal() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sealed {
		return b.err
	}
	b.sealed = true
	if err := b.validateLocked(); err != nil {
		b.err = err
		return err
	}
	b.buildAndRegisterLocked()
	return nil
}

func (b *JobBuilder) buildAndRegisterLocked() *JobDefinition {
	if b.definition == nil {
		return nil
	}
	b.definition.ParamBindings = coreadapter.CopyIntStringMap(b.paramBindings)
	b.routes.reg.RegisterDecl(AdapterID, "jobs", b.definition)
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
