---
title: Scheduler
weight: 11
prev: /docs/reference/mq
next: /docs/reference/integrations
---

包：`github.com/sao-lang/lania-g/protocol/scheduler/v3`

## 1. Adapter

```go
schedAdp := scheduleradapter.New()

app, _ := application.New(root, application.Options{
	Registry: registry.New(),
}, httpAdp, schedAdp)
```

| 方法 | 说明 |
|------|------|
| `New()` | 创建 adapter |
| `API()` | DSL 上下文 |
| `Snapshot()` | `map[string]JobSnapshot` 运行快照 |
| `SetUniqueLocker(locker)` | 替换唯一执行锁（默认进程内） |
| `SetHistoryLimit(n)` | 保留的最近运行记录条数 |
| `MountHTTPBridge(host, adp, pattern)` | 暴露 HTTP 观测端点 |

## 2. 任务 DSL

```go
schedAPI := schedAdp.API().(*scheduleradapter.API)

schedAPI.Job("cleanup", worker).
	Cron("0 3 * * *", worker.Cleanup).   // 每天 03:00
	Retry(3, 5*time.Second).
	MaxConcurrency(1).
	Unique("cleanup").
	WithTimeout(30 * time.Second).
	Misfire(scheduleradapter.MisfireSkip).
	BuildE()

schedAPI.Job("tick", worker).
	Every(100 * time.Millisecond, worker.Tick).
	BuildE()
```

| Builder | 方法 | 说明 |
|---------|------|------|
| `API` | `Job(name, receiver)` | 开始声明 |
| `JobBuilder` | `Cron(expr, handler)` | cron 触发 |
| | `Every(d, handler)` | 固定间隔重复 |
| | `After(d, handler)` | 延迟一次 |
| | `Retry(attempts, backoff)` | 失败重试 |
| | `MaxConcurrency(n)` | 同任务并发上限 |
| | `Unique(key...)` | 唯一执行（同 key 不并发） |
| | `WithTimeout(d)` | 单次执行超时 |
| | `Misfire(policy)` | 错过触发点策略（`MisfireQueue` / `MisfireSkip`） |
| | `BindParam(index, name)` | 参数绑定名 |
| | `Build()/BuildE()/Err()` | 收尾 |

`JobDefinition` 关键字段：

```go
type JobDefinition struct {
	Name           string
	TriggerKind    TriggerKind      // cron / every / after
	Expression     string
	Duration       time.Duration
	Receiver       any
	HandlerName    string
	MaxConcurrency int
	RetryAttempts  int
	RetryBackoff   time.Duration
	Unique         bool
	UniqueKey      string
	Timeout        time.Duration
	MisfirePolicy  MisfirePolicy
	ParamBindings  map[int]string
}
```

## 3. cron 表达式支持范围

> **重要**：内置的 cron 解析器是轻量实现，不同于 `robfig/cron`。

| 写法 | 是否支持 |
|------|----------|
| 5 段（分 时 日 月 周） | ✅ |
| 6 段（秒 分 时 日 月 周） | ✅ |
| `@every 30s` | ✅ |
| `*` | ✅ |
| `*/n` | ✅ |
| `a-b` 区间 | ✅ |
| `a,b,c` 枚举 | ✅ |
| `a-b/n`（区间步长） | ❌ |
| `MON`/`JAN` 名称 | ❌ |
| `?` `L` `W` `#` | ❌ |

非法表达式在 `Start()` 阶段报错（`invalid cron expression: ...`）。

## 4. 处理器签名

```go
func (w *Worker) Cleanup(
	ctx stdctx.Context,
	name schedulerbinding.JobName,
	trigger schedulerbinding.TriggerType,
	runID schedulerbinding.RunID,
	scheduledAt schedulerbinding.ScheduledAt,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}
```

| wrapper | 含义 |
|---------|------|
| `Context` | `context.Context`（含超时/取消） |
| `JobName` | 任务名 |
| `TriggerType` | `after` / `every` / `cron` |
| `RunID` | 本次运行标识 |
| `ScheduledAt` | 计划执行时间 |

「参数名绑定」用 `BindParam(index, name)`，配合业务自定义 pipe 使用。

## 5. 执行观测

```go
if err := scheduleradapter.MountHTTPBridge(app, schedAdp, "/scheduler/status"); err != nil {
	panic(err)
}
```

```bash
curl http://127.0.0.1:8080/scheduler/status
```

也可以直接读内存快照：

```go
for name, snap := range schedAdp.Snapshot() {
	fmt.Println(name, snap.Metrics.TotalRuns, snap.Metrics.SuccessRuns, snap.Metrics.LastError)
}
```

快照结构：

```go
type JobSnapshot struct {
	Definition *JobDefinition `json:"definition,omitempty"`
	Metrics    JobMetrics     `json:"metrics"`
	RecentRuns []JobRunRecord `json:"recentRuns,omitempty"`
}

type JobMetrics struct {
	JobName        string
	Trigger        TriggerKind
	TotalRuns      int64
	SuccessRuns    int64
	FailureRuns    int64
	LastStartedAt  time.Time
	LastFinishedAt time.Time
	LastDuration   time.Duration
	LastError      string
	NextRunAt      time.Time
}
```

`RecentRuns` 长度由 `SetHistoryLimit(n)` 控制。

## 6. 唯一执行与分布式

```go
type UniqueLocker interface {
	TryLock(key string) bool
	Unlock(key string)
}
```

| 实现 | 说明 |
|------|------|
| `NewInMemoryUniqueLocker()` | 默认，仅单进程 |
| 自定义实现 | Redis `SETNX` / DB 行锁，实现跨实例协调 |

```go
schedAdp.SetUniqueLocker(redisLocker)
```

## 7. 实践建议

- 周期任务不要长时间阻塞；配合 `WithTimeout` 与 `MaxConcurrency`
- `Every` 间隔小于任务耗时会导致堆积，必要时用 `Unique` 保护
- 需要跨实例唯一时，必须替换 `UniqueLocker`
- 错过触发点：`MisfireQueue` 补执行，`MisfireSkip` 直接跳过
- `After` 只执行一次，适合启动后的延迟初始化任务
