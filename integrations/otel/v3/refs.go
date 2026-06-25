// refs.go 定义 otel integration 的命名引用 wrapper 与 binding 注册辅助。
package otel

import (
	"fmt"
	"reflect"

	coredi "github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// TelemetryNamer 允许 marker 类型自定义 `TelemetryRef` 对应的实例名。
type TelemetryNamer interface {
	TelemetryName() string
}

// InjectTelemetry 用于在 handler 参数中注入默认 Telemetry 实例。
type InjectTelemetry struct {
	*Telemetry
}

// TelemetryRef 用于在 handler 参数中注入“按名字区分”的 Telemetry 实例。
//
// N 仅作为 marker 类型参与命名推导（见 TelemetryNamer）。
type TelemetryRef[N any] struct {
	*Telemetry
	_ *N
}

// InjectTraceContext 用于在 handler 参数中注入当前 TraceContext 快照。
type InjectTraceContext struct {
	TraceContext
}

// InjectRequestID 用于在 handler 参数中注入 request id。
type InjectRequestID struct {
	Value string
}

// TelemetryToken 返回按 name 区分的 Telemetry 实例 token。
func TelemetryToken(name string) string {
	if name == "" {
		name = DefaultName
	}
	return "otel:instance:" + name
}

// RegisterBindings 注册 OTel 相关的 binding resolver。
func RegisterBindings(reg *registry.Registry) {
	if reg == nil {
		RegisterBindingsCompat()
		return
	}
	registerBindings(reg)
}

// RegisterBindingsCompat 显式保留“写入全局 registry”的兼容 binding 注册入口。
func RegisterBindingsCompat() {
	registerBindings(registry.GlobalWithUsage("integrations/otel.RegisterBindingsCompat"))
}

func registerBindings(reg *registry.Registry) {
	reg.RegisterBindings(runtime.NewBindingResolvers(
		registration("OTelInject", coreintegration.MatchNamedWrapper(packagePath(), "InjectTelemetry"), resolveInjectTelemetry),
		registration("OTelRef", coreintegration.MatchNamedWrapper(packagePath(), "TelemetryRef"), resolveTelemetryRef),
		registration("OTelTraceContext", coreintegration.MatchNamedWrapper(packagePath(), "InjectTraceContext"), resolveTraceContext),
		registration("OTelRequestID", coreintegration.MatchNamedWrapper(packagePath(), "InjectRequestID"), resolveRequestID),
	)...)
}

func resolveInjectTelemetry(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	value, err := resolveNamedTelemetry(ctx, DefaultName)
	if err != nil {
		return nil, err
	}
	return coreintegration.WrapFirstField(desc.WrapperType, reflect.ValueOf(value))
}

func resolveTelemetryRef(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	value, err := resolveNamedTelemetry(ctx, telemetryNameFromWrapper(desc.WrapperType))
	if err != nil {
		return nil, err
	}
	return coreintegration.WrapFirstField(desc.WrapperType, reflect.ValueOf(value))
}

func resolveTraceContext(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	return coreintegration.WrapFirstField(desc.WrapperType, reflect.ValueOf(Current(ctx)))
}

func resolveRequestID(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	return coreintegration.WrapFirstField(desc.WrapperType, reflect.ValueOf(Current(ctx).RequestID))
}

func resolveNamedTelemetry(ctx *runtime.HandlerContext, name string) (*Telemetry, error) {
	if ctx == nil || ctx.Container == nil {
		return nil, fmt.Errorf("otel binding requires request container")
	}
	if value, err := ctx.Container.Get(TelemetryToken(name)); err == nil {
		if telemetry, ok := value.(*Telemetry); ok {
			return telemetry, nil
		}
	}
	value, err := coredi.GetByType[*Telemetry](ctx.Container)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func telemetryNameFromWrapper(wrapperType reflect.Type) string {
	return coreintegration.ResolveMarkerName(wrapperType, DefaultName, func(marker any) (string, bool) {
		namer, ok := marker.(TelemetryNamer)
		if !ok {
			return "", false
		}
		return namer.TelemetryName(), true
	})
}

func registration(name string, match func(reflect.Type) (runtime.WrapperDescriptor, bool), resolve func(*runtime.HandlerContext, runtime.WrapperDescriptor) (any, error)) runtime.BindingRegistration {
	return runtime.BindingRegistration{
		Name:    name,
		Match:   match,
		Resolve: resolve,
	}
}

func packagePath() string {
	return reflect.TypeFor[TelemetryNamer]().PkgPath()
}
