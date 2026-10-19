---
title: Scheduler
weight: 6
prev: /docs/protocols/mq
next: /docs/integrations
---

包：`github.com/sao-lang/lania-g/protocol/scheduler/v3`

## 1. 创建 adapter

```go
schedAdapter := scheduleradapter.New()

app, _ := application.NewWithOptions(root, opts, httpAdapter, schedAdapter)
```

## 2. 声明任务

```go
schedAPI := schedAdapter.API().(*scheduleradapter.API)

schedAPI.Job("tick", worker).
	Every(100 * time.Millisecond, worker.Tick).
	BuildE()

schedAPI.Job("cleanup", worker).
	Cron("0 3 * * *", worker.Cleanup). // 每天 03:00
	Retry(3, 5*time.Second).
	MaxConcurrency(1).
	Unique().
	Misfire(scheduleradapter.MisfireSkip).
	BuildE()
```

| 触发方式 | 说明 |
|----------|------|
| `After(d)` | 延迟执行一次 |
| `Every(d)` | 周期执行 |
| `Cron(expr)` | cron 表达式 |

## 3. 处理器

```go
func (w *Worker) Tick(
	ctx context.Context,
	name schedulerbinding.JobName,
	trigger schedulerbinding.TriggerType,
) error {
	// 业务逻辑
	return nil
}
```

## 4. 重试与并发

```go
Job("retry-job", worker).
	After(10*time.Millisecond, worker.Run).
	Retry(2, 5*time.Millisecond).
	BuildE()
```

| 配置 | 作用 |
|------|------|
| `Retry(attempts, backoff)` | 失败重试 |
| `MaxConcurrency(n)` | 单任务并发上限 |
| `Unique(key...)` | 分布式/进程内唯一执行 |
| `WithTimeout(d)` | 单次执行超时 |
| `Misfire(policy)` | 错过触发点的处理策略 |

## 5. 运行快照

```go
if err := scheduleradapter.MountHTTPBridge(app, schedAdapter, "/scheduler/status"); err != nil {
	panic(err)
}
```

访问：

```bash
curl http://127.0.0.1:8080/scheduler/status
```

返回每个 job 的定义与运行统计，可用于观测与排障。

## 6. 绑定类型

| wrapper | 含义 |
|---------|------|
| `JobName` | 任务名 |
| `TriggerType` | 触发方式（after/every/cron） |
| `RunID` | 本次运行标识 |
| `ScheduledAt` | 计划执行时间 |

## 7. 注意事项

- 周期任务不要做长时间阻塞操作，必要时配合 `MaxConcurrency` 与 `WithTimeout`
- 需要跨进程唯一时，`Unique` 依赖底层存储/锁实现，单机示例中为进程内语义
- 任务失败会记录到快照，便于定位
