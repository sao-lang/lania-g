// refs.go 定义 logger integration 的命名引用 wrapper 与 binding 注册辅助。
package logger

import (
	"fmt"
	"reflect"

	coredi "github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// LoggerNamer 约定命名 logger 引用的名称来源。
type LoggerNamer interface {
	LoggerName() string
}

// DefaultLogger 用于标记默认 logger。
type DefaultLogger struct{}

// LoggerName 返回默认 logger 的名称。
func (DefaultLogger) LoggerName() string { return "default" }

// InjectLogger 表示注入默认 logger 的包装类型。
type InjectLogger struct {
	Logger
}

// LoggerRef 表示按名称注入 logger 的包装类型。
type LoggerRef[N any] struct {
	Logger
	_ *N
}

// LoggerToken 返回某个命名 logger 对应的 DI token。
func LoggerToken(name string) string {
	if name == "" {
		name = "default"
	}
	return "logger:instance:" + name
}

// RegisterBindings 注册 logger 相关的注入包装 binding。
func RegisterBindings(reg *registry.Registry) {
	if reg == nil {
		RegisterBindingsCompat()
		return
	}
	registerBindings(reg)
}

// RegisterBindingsCompat 显式保留“写入全局 registry”的兼容 binding 注册入口。
func RegisterBindingsCompat() {
	registerBindings(registry.GlobalWithUsage("integrations/logger.RegisterBindingsCompat"))
}

func registerBindings(reg *registry.Registry) {
	reg.RegisterBindings(runtime.NewBindingResolvers(
		registration("LoggerInject", coreintegration.MatchNamedWrapper(packagePath(), "InjectLogger"), resolveInjectLogger),
		registration("LoggerRef", coreintegration.MatchNamedWrapper(packagePath(), "LoggerRef"), resolveLoggerRef),
	)...)
}

func resolveInjectLogger(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	value, err := resolveNamedLogger(ctx, "default")
	if err != nil {
		return nil, err
	}
	return coreintegration.WrapFirstField(desc.WrapperType, reflect.ValueOf(value))
}

func resolveLoggerRef(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	value, err := resolveNamedLogger(ctx, loggerNameFromWrapper(desc.WrapperType))
	if err != nil {
		return nil, err
	}
	return coreintegration.WrapFirstField(desc.WrapperType, reflect.ValueOf(value))
}

func resolveNamedLogger(ctx *runtime.HandlerContext, name string) (Logger, error) {
	if ctx == nil || ctx.Container == nil {
		return nil, fmt.Errorf("logger binding requires request container")
	}
	if value, err := ctx.Container.Get(LoggerToken(name)); err == nil {
		if logger, ok := value.(Logger); ok {
			return logger, nil
		}
	}
	value, err := coredi.GetByType[Logger](ctx.Container)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func loggerNameFromWrapper(wrapperType reflect.Type) string {
	return coreintegration.ResolveMarkerName(wrapperType, "default", func(marker any) (string, bool) {
		namer, ok := marker.(LoggerNamer)
		if !ok {
			return "", false
		}
		return namer.LoggerName(), true
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
	return reflect.TypeFor[LoggerNamer]().PkgPath()
}
