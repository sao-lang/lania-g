// register.go 注册 Scheduler 协议的默认 binding 声明与 compat 入口。
package scheduler

import (
	stdctx "context"
	"fmt"
	"reflect"
	"time"

	coreregistry "github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	schedulerprotocol "github.com/sao-lang/lania-g/protocol/scheduler/v3/protocol"
)

const (
	// MetadataKeyJobName 是 metadata 中保存任务名的键。
	MetadataKeyJobName     = "scheduler.job_name"
	// MetadataKeyTriggerType 是 metadata 中保存触发器类型的键。
	MetadataKeyTriggerType = "scheduler.trigger_type"
	// MetadataKeyRunID 是 metadata 中保存本次执行 ID 的键。
	MetadataKeyRunID       = "scheduler.run_id"
	// MetadataKeyScheduledAt 是 metadata 中保存调度时间的键。
	MetadataKeyScheduledAt = "scheduler.scheduled_at"
)

// RegisterDefaults 将内置的 Scheduler 参数绑定规则注册到 runtime。
func RegisterDefaults(rt *runtime.Runtime) {
	for _, reg := range DefaultRegistrations() {
		rt.RegisterBinding(runtime.NewBindingResolver(reg))
	}
}

// RegisterDefaultsToRegistry 将内置的 Scheduler 参数绑定规则注册到 registry。
// 如果 reg 为空，则回退到全局 registry。
func RegisterDefaultsToRegistry(reg *coreregistry.Registry) {
	if reg == nil {
		RegisterDefaultsCompat()
		return
	}
	registerDefaultsToRegistry(reg)
}

// RegisterDefaultsCompat 显式保留“写入全局 registry”的兼容 binding 注册入口。
func RegisterDefaultsCompat() {
	registerDefaultsToRegistry(coreregistry.GlobalWithUsage("binding/scheduler.RegisterDefaultsCompat"))
}

func registerDefaultsToRegistry(reg *coreregistry.Registry) {
	reg.RegisterBindings(DefaultResolvers()...)
}

// DefaultRegistrations 返回 Scheduler 协议默认启用的一组 binding registration。
//
// Scheduler 的 binding 很克制：只暴露作业调度上下文本身，
// 不做复杂 body/header 解码，因为任务触发输入主要来自调度器元数据。
func DefaultRegistrations() []runtime.BindingRegistration {
	allowed := map[runtime.Protocol]bool{schedulerprotocol.Protocol: true}
	return []runtime.BindingRegistration{
		registration("HandlerContext", nil, matchHandlerContext, resolveHandlerContext),
		registration("Context", allowed, matchStdContext, resolveStdContext),
		registration("JobName", allowed, matchNamedType[JobName]("JobName"), resolveJobName),
		registration("TriggerType", allowed, matchNamedType[TriggerType]("TriggerType"), resolveTriggerType),
		registration("RunID", allowed, matchNamedType[RunID]("RunID"), resolveRunID),
		registration("ScheduledAt", allowed, matchNamedType[ScheduledAt]("ScheduledAt"), resolveScheduledAt),
	}
}

// DefaultResolvers 返回 Scheduler 协议默认启用的一组 binding resolver。
func DefaultResolvers() []runtime.BindingResolver {
	return runtime.NewBindingResolvers(DefaultRegistrations()...)
}

// registration 是局部薄封装，负责把 matcher 与 resolver 配对。
func registration(name string, allowed map[runtime.Protocol]bool, match func(reflect.Type) (runtime.WrapperDescriptor, bool), resolve func(*runtime.HandlerContext, runtime.WrapperDescriptor) (any, error)) runtime.BindingRegistration {
	return runtime.BindingRegistration{
		Name:             name,
		AllowedProtocols: allowed,
		Match:            match,
		Resolve:          resolve,
	}
}

// matchHandlerContext 允许 handler 直接拿到底层 runtime.HandlerContext。
func matchHandlerContext(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	ctxPtr := reflect.TypeFor[*runtime.HandlerContext]()
	if t == ctxPtr {
		return runtime.WrapperDescriptor{Kind: "HandlerContext", WrapperType: t, InnerType: t}, true
	}
	return runtime.WrapperDescriptor{}, false
}

func resolveHandlerContext(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	return ctx, nil
}

// matchStdContext 匹配标准库 `context.Context`，方便 job 直接复用取消/超时语义。
func matchStdContext(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	ctxIface := reflect.TypeFor[stdctx.Context]()
	if t == ctxIface {
		return runtime.WrapperDescriptor{Kind: "Context", WrapperType: t, InnerType: t}, true
	}
	return runtime.WrapperDescriptor{}, false
}

func resolveStdContext(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if ctx == nil {
		return stdctx.Background(), nil
	}
	return ctx.Context(), nil
}

func matchNamedType[T any](name string) func(reflect.Type) (runtime.WrapperDescriptor, bool) {
	base := reflect.TypeFor[T]()
	return func(t reflect.Type) (runtime.WrapperDescriptor, bool) {
		if t != base {
			return runtime.WrapperDescriptor{}, false
		}
		return runtime.WrapperDescriptor{Kind: name, WrapperType: t, InnerType: t}, true
	}
}

// 下面几项 resolver 都只是把 scheduler adapter 在触发时写入的 metadata
// 投影成命名类型，供业务 job 显式声明依赖。
func resolveJobName(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyJobName); ok {
		if s, ok2 := v.(string); ok2 {
			return JobName(s), nil
		}
	}
	return JobName(""), nil
}

func resolveTriggerType(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyTriggerType); ok {
		if s, ok2 := v.(string); ok2 {
			return TriggerType(s), nil
		}
	}
	return TriggerType(""), nil
}

func resolveRunID(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyRunID); ok {
		if s, ok2 := v.(string); ok2 {
			return RunID(s), nil
		}
	}
	return RunID(""), nil
}

func resolveScheduledAt(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyScheduledAt); ok {
		switch vv := v.(type) {
		case time.Time:
			return ScheduledAt(vv), nil
		case ScheduledAt:
			return vv, nil
		default:
			// 这里不做宽松字符串解析，避免把错误的调度器元数据默默吞成零值时间。
			return nil, fmt.Errorf("invalid scheduled_at metadata type: %T", v)
		}
	}
	return ScheduledAt(time.Time{}), nil
}
