// observe.go 提供 Scheduler adapter 的执行观测与事件回调辅助。
package scheduler

import (
	"sync"
	"time"
)

// UniqueLocker 定义定时任务“唯一执行”所需的最小加锁能力。
// scheduler 只依赖 TryLock/Unlock 这两个最小接口，因此后续可以替换成 Redis/DB 等分布式实现。
type UniqueLocker interface {
	TryLock(key string) bool
	Unlock(key string)
}

// inMemoryUniqueLocker 是默认的进程内唯一执行锁。
// 它只保证单进程内不重入，不提供跨实例协调能力。
type inMemoryUniqueLocker struct {
	mu    sync.Mutex
	locks map[string]bool
}

// NewInMemoryUniqueLocker 创建一个基于内存的唯一执行锁实现。
func NewInMemoryUniqueLocker() UniqueLocker {
	return &inMemoryUniqueLocker{locks: make(map[string]bool)}
}

// TryLock 尝试对某个 key 加锁；成功返回 true，失败返回 false。
func (l *inMemoryUniqueLocker) TryLock(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.locks[key] {
		return false
	}
	l.locks[key] = true
	return true
}

// Unlock 释放某个 key 对应的唯一执行锁。
func (l *inMemoryUniqueLocker) Unlock(key string) {
	l.mu.Lock()
	delete(l.locks, key)
	l.mu.Unlock()
}

// JobRunRecord 描述一次任务执行记录。
// 它是 history 与快照接口共享的统一结构。
type JobRunRecord struct {
	RunID       string        `json:"runId"`
	JobName     string        `json:"jobName"`
	Trigger     TriggerKind   `json:"trigger"`
	ScheduledAt time.Time     `json:"scheduledAt"`
	StartedAt   time.Time     `json:"startedAt"`
	FinishedAt  time.Time     `json:"finishedAt"`
	Duration    time.Duration `json:"duration"`
	Success     bool          `json:"success"`
	Attempts    int           `json:"attempts"`
	Error       string        `json:"error,omitempty"`
}

// JobMetrics 描述某个任务的聚合指标快照。
// 它不存完整历史，只存聚合计数与最近一次执行结果。
type JobMetrics struct {
	JobName        string        `json:"jobName"`
	Trigger        TriggerKind   `json:"trigger"`
	TotalRuns      int64         `json:"totalRuns"`
	SuccessRuns    int64         `json:"successRuns"`
	FailureRuns    int64         `json:"failureRuns"`
	LastStartedAt  time.Time     `json:"lastStartedAt"`
	LastFinishedAt time.Time     `json:"lastFinishedAt"`
	LastDuration   time.Duration `json:"lastDuration"`
	LastError      string        `json:"lastError,omitempty"`
	NextRunAt      time.Time     `json:"nextRunAt"`
}

// JobSnapshot 描述某个任务当前的定义、指标与最近运行记录。
// Snapshot API 最终就是把每个 job 的这三部分拼在一起返回。
type JobSnapshot struct {
	Definition *JobDefinition `json:"definition,omitempty"`
	Metrics    JobMetrics     `json:"metrics"`
	RecentRuns []JobRunRecord `json:"recentRuns,omitempty"`
}
