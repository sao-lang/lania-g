// defs.go 定义 Scheduler adapter 在 registry 与运行期使用的声明结构。
package scheduler

import (
	"reflect"
	"time"

	"github.com/sao-lang/lania-g/kernel/v3/di"
)

// TriggerKind 表示定时任务触发器类型。
type TriggerKind string

const (
	// TriggerCron 表示按 cron 表达式触发。
	TriggerCron  TriggerKind = "cron"
	// TriggerEvery 表示按固定间隔重复触发。
	TriggerEvery TriggerKind = "every"
	// TriggerAfter 表示在指定延迟后触发一次。
	TriggerAfter TriggerKind = "after"
)

// MisfirePolicy 表示错过调度时的处理策略。
type MisfirePolicy string

const (
	// MisfireQueue 表示错过调度时补入执行队列。
	MisfireQueue MisfirePolicy = "queue"
	// MisfireSkip 表示错过调度时直接跳过。
	MisfireSkip  MisfirePolicy = "skip"
)

// JobDefinition 是一条调度任务的编译期声明。
// 它既包含“什么时候触发”，也包含“由哪个 receiver 方法执行”以及“失败时怎么重试/限流”。
type JobDefinition struct {
	Name string

	// TriggerKind + Expression/Duration 共同决定触发方式。
	// - cron: 主要看 Expression
	// - every/after: 主要看 Duration
	TriggerKind TriggerKind
	Expression  string
	Duration    time.Duration

	// Receiver/HandlerName 只描述执行目标，真正运行时实例仍由容器解析。
	Receiver    any
	HandlerName string

	// 下面这些字段描述调度期控制策略，而不是业务 handler 本身：
	// 并发、重试、唯一执行、超时、misfire 策略等。
	MaxConcurrency int
	RetryAttempts  int
	RetryBackoff   time.Duration
	Unique         bool
	UniqueKey      string
	Timeout        time.Duration
	MisfirePolicy  MisfirePolicy

	// ParamBindings 记录“参数索引 -> binding 名称”，供编译阶段生成 ParamPlan。
	ParamBindings map[int]string
}

// routeOwnership 是 scheduler plugin 在 Scan 阶段附加的模块 owner 信息。
type routeOwnership struct {
	definition    *JobDefinition
	moduleKey     string
	moduleType    reflect.Type
	receiverToken reflect.Type
	container     *di.Container
}
