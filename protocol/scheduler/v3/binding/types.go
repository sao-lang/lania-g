// types.go 定义 Scheduler 协议暴露给 handler 的 binding wrapper 与辅助类型。
package scheduler

import (
	stdctx "context"
	"time"
)

// Context 表示标准库的 `context.Context`，可通过 binding 注入到任务处理函数。
// 调度器在 runJob 时会把 timeout/cancel 包装进这个 context 传给业务 job。
type Context = stdctx.Context

// JobName 表示当前任务名称。
type JobName string

// TriggerType 表示触发类型，例如 cron、every、after、manual 等。
type TriggerType string

// RunID 表示本次执行的唯一标识。
type RunID string

// ScheduledAt 表示计划触发时间。
// 它和真实开始执行时间可能不同，尤其是在重试、排队或 limiter 挤压时。
type ScheduledAt time.Time
