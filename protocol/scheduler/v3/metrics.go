// metrics.go 维护 Scheduler adapter 的运行指标与快照辅助。
package scheduler

import (
	coreadapter "github.com/sao-lang/lania-g/kernel/v3/adapter"
	"time"
)

// Snapshot 返回当前 scheduler 的运行期快照（定义、指标与最近运行记录）。
// 这里返回的是只读副本视图，避免调用方拿到内部 map/slice 后直接改运行期状态。
func (a *Adapter) Snapshot() map[string]JobSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]JobSnapshot, len(a.definitions))
	for key, def := range a.definitions {
		snap := JobSnapshot{}
		if def != nil {
			snap.Definition = cloneJobDefinition(def)
		}
		if metric := a.metrics[key]; metric != nil {
			snap.Metrics = *metric
		}
		if history := a.history[key]; len(history) > 0 {
			snap.RecentRuns = append([]JobRunRecord{}, history...)
		}
		out[key] = snap
	}
	return out
}

// recordRun 在一次 job 执行结束后统一更新指标、历史和 next-run 估计。
// 它是 scheduler 观测面的收口点：无论成功、失败、重试结束，最后都应该走到这里。
func (a *Adapter) recordRun(def *JobDefinition, record JobRunRecord) {
	key := a.limiterKey(def)
	a.mu.Lock()
	defer a.mu.Unlock()
	metric := a.metrics[key]
	if metric == nil {
		metric = &JobMetrics{JobName: def.Name, Trigger: def.TriggerKind}
		a.metrics[key] = metric
	}
	metric.TotalRuns++
	metric.LastStartedAt = record.StartedAt
	metric.LastFinishedAt = record.FinishedAt
	metric.LastDuration = record.Duration
	metric.LastError = record.Error
	if record.Success {
		metric.SuccessRuns++
	} else {
		metric.FailureRuns++
	}
	// 新记录始终插到最前面，保证 RecentRuns 按“最近一次优先”排列。
	history := append([]JobRunRecord{record}, a.history[key]...)
	if len(history) > a.historyLimit {
		history = history[:a.historyLimit]
	}
	a.history[key] = history
	a.updateNextRunLocked(def, record.FinishedAt)
}

// ensureMetrics 确保某个 job 对应的聚合指标结构已初始化。
func (a *Adapter) ensureMetrics(def *JobDefinition) {
	key := a.limiterKey(def)
	if _, ok := a.metrics[key]; !ok {
		a.metrics[key] = &JobMetrics{JobName: def.Name, Trigger: def.TriggerKind}
	}
}

// updateNextRun/updateNextRunLocked 根据当前定义和基准时间刷新下一次预估触发时间。
func (a *Adapter) updateNextRun(def *JobDefinition, base time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.updateNextRunLocked(def, base)
}

func (a *Adapter) updateNextRunLocked(def *JobDefinition, base time.Time) {
	key := a.limiterKey(def)
	metric := a.metrics[key]
	if metric == nil {
		metric = &JobMetrics{JobName: def.Name, Trigger: def.TriggerKind}
		a.metrics[key] = metric
	}
	metric.NextRunAt = estimateNextRun(def, base)
}

// estimateNextRun 只做“尽力而为”的下一次运行时间估计，
// 主要用于观测展示，不承诺和实际调度器内部时钟完全一致。
func estimateNextRun(def *JobDefinition, base time.Time) time.Time {
	if def == nil {
		return time.Time{}
	}
	switch def.TriggerKind {
	case TriggerAfter, TriggerEvery:
		if def.Duration > 0 {
			return base.Add(def.Duration)
		}
	case TriggerCron:
		return estimateNextCronRun(def.Expression, base)
	}
	return time.Time{}
}

// cloneJobDefinition 为快照/观测输出复制一份定义，避免把内部 map 直接暴露出去。
func cloneJobDefinition(def *JobDefinition) *JobDefinition {
	if def == nil {
		return nil
	}
	out := *def
	out.ParamBindings = coreadapter.CopyIntStringMap(def.ParamBindings)
	return &out
}
