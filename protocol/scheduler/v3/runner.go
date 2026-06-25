// runner.go 实现 Scheduler adapter 的任务执行与调度驱动。
package scheduler

import (
	stdctx "context"
	"fmt"
	"strings"
	"time"

	schedulerbinding "github.com/sao-lang/lania-g/protocol/scheduler/v3/binding"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	schedulerprotocol "github.com/sao-lang/lania-g/protocol/scheduler/v3/protocol"
)

// runCron 负责 cron 风格触发。
// 它同时兼容两类表达式：
// - `@every <duration>`：退化为固定间隔 ticker
// - 标准 cron 表达式：按秒轮询并用 slot 去重
func (a *Adapter) runCron(ctx stdctx.Context, def *JobDefinition) {
	if everyExpr, ok := strings.CutPrefix(def.Expression, "@every "); ok {
		duration, err := time.ParseDuration(strings.TrimSpace(everyExpr))
		if err != nil || duration <= 0 {
			return
		}
		ticker := time.NewTicker(duration)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				a.updateNextRun(def, t)
				a.dispatchJob(ctx, def, t)
			}
		}
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastTrigger := int64(0)
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			if matchesCronExpression(def.Expression, t) {
				slot := cronSlot(def.Expression, t)
				if slot != lastTrigger {
					// cron 轮询是按秒扫描的，这里用 slot 去重，避免同一时间槽内重复触发。
					lastTrigger = slot
					a.updateNextRun(def, t)
					a.dispatchJob(ctx, def, t)
				}
			}
		}
	}
}

// runTicker 负责 fixed-interval 调度。
func (a *Adapter) runTicker(ctx stdctx.Context, def *JobDefinition) {
	ticker := time.NewTicker(def.Duration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			a.updateNextRun(def, t)
			a.dispatchJob(ctx, def, t)
		}
	}
}

// runTimer 负责一次性延迟触发。
func (a *Adapter) runTimer(ctx stdctx.Context, def *JobDefinition) {
	timer := time.NewTimer(def.Duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case t := <-timer.C:
		a.updateNextRun(def, t)
		a.dispatchJob(ctx, def, t)
	}
}

// dispatchJob 只负责“是否允许这次触发进入执行”：
// - 先检查 limiter/并发槽
// - 真正执行放到独立 goroutine 的 runJob
func (a *Adapter) dispatchJob(ctx stdctx.Context, def *JobDefinition, scheduledAt time.Time) {
	if def == nil || a.host == nil || !a.acquireSlot(def) {
		return
	}
	go a.runJob(ctx, def, scheduledAt)
}

// runJob 把一次调度触发投影成统一的 HandlerContext，并处理 timeout/retry/history 记录。
func (a *Adapter) runJob(ctx stdctx.Context, def *JobDefinition, scheduledAt time.Time) {
	defer a.releaseSlot(def)
	rctx := runtime.AcquireHandlerContext(schedulerprotocol.Protocol)
	defer runtime.ReleaseHandlerContext(rctx)
	if ctx == nil {
		ctx = stdctx.Background()
	}
	if def.Timeout > 0 {
		var cancel stdctx.CancelFunc
		ctx, cancel = stdctx.WithTimeout(ctx, def.Timeout)
		defer cancel()
	}
	rctx.WithContext(ctx)
	rctx.Request.Method = string(def.TriggerKind)
	rctx.Request.Path = def.Name
	rctx.Set(schedulerbinding.MetadataKeyJobName, def.Name)
	rctx.Set(schedulerbinding.MetadataKeyTriggerType, string(def.TriggerKind))
	rctx.Set(schedulerbinding.MetadataKeyScheduledAt, scheduledAt)
	routeKey := runtime.BuildRouteKey(schedulerprotocol.Protocol, string(def.TriggerKind), def.Name)
	rctx.RouteKey = routeKey
	runID := fmt.Sprintf("%s-%d", def.Name, scheduledAt.UnixNano())
	startedAt := time.Now()
	rctx.Set(schedulerbinding.MetadataKeyRunID, runID)
	attempts := def.RetryAttempts + 1
	if attempts <= 0 {
		attempts = 1
	}
	var finalErr error
	actualAttempts := 0
	for attempt := 0; attempt < attempts; attempt++ {
		actualAttempts++
		_, err := a.host.Runtime().Execute(rctx)
		if err == nil {
			// 成功时立即落成功记录，并结束整个重试流程。
			a.recordRun(def, JobRunRecord{RunID: runID, JobName: def.Name, Trigger: def.TriggerKind, ScheduledAt: scheduledAt, StartedAt: startedAt, FinishedAt: time.Now(), Duration: time.Since(startedAt), Success: true, Attempts: actualAttempts})
			return
		}
		finalErr = err
		if attempt+1 >= attempts {
			// 所有尝试都失败后，记录最终失败结果。
			a.recordRun(def, JobRunRecord{RunID: runID, JobName: def.Name, Trigger: def.TriggerKind, ScheduledAt: scheduledAt, StartedAt: startedAt, FinishedAt: time.Now(), Duration: time.Since(startedAt), Success: false, Attempts: actualAttempts, Error: err.Error()})
			return
		}
		if def.RetryBackoff > 0 {
			select {
			case <-ctx.Done():
				// 若在 backoff 等待期间被取消，则按最终失败收口，错误沿用最近一次执行错误。
				a.recordRun(def, JobRunRecord{RunID: runID, JobName: def.Name, Trigger: def.TriggerKind, ScheduledAt: scheduledAt, StartedAt: startedAt, FinishedAt: time.Now(), Duration: time.Since(startedAt), Success: false, Attempts: actualAttempts, Error: finalErr.Error()})
				return
			case <-time.After(def.RetryBackoff):
			}
		}
	}
}
