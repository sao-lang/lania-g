// refs.go 定义 config integration 的命名引用 wrapper 与 binding 注册辅助。
package config

import (
	"fmt"
	"reflect"

	coredi "github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// Section 表示一个需要按 section 解码的配置片段。
type Section[T any] struct {
	Value T
}

// Value 表示一个需要按单值读取的配置项。
type Value[T any] struct {
	Value T
}

// MustSection 读取并解码一个配置 section；失败时 panic。
func MustSection[T any](loader *Loader, key string) T {
	value, err := ReadSection[T](loader, key)
	if err != nil {
		panic(err)
	}
	return value
}

// ReadSection 读取并解码一个配置 section。
func ReadSection[T any](loader *Loader, key string) (T, error) {
	var out T
	if loader == nil {
		return out, fmt.Errorf("config loader is nil")
	}
	if key == "" {
		err := loader.Unmarshal("", &out)
		return out, err
	}
	err := loader.Unmarshal(key, &out)
	return out, err
}

// ReadValue 读取单个配置值，并尽量转换为目标类型。
func ReadValue[T any](loader *Loader, key string) (T, error) {
	var out T
	if loader == nil {
		return out, fmt.Errorf("config loader is nil")
	}
	value, ok := loader.Get(key)
	if !ok {
		return out, fmt.Errorf("config key not found: %s", key)
	}
	rv := reflect.ValueOf(value)
	target := reflect.TypeFor[T]()
	if rv.IsValid() && rv.Type().AssignableTo(target) {
		return value.(T), nil
	}
	// 回退到 `Unmarshal`，以保持标量转换行为一致。
	err := loader.Unmarshal(key, &out)
	return out, err
}

// RegisterBindings 注册配置读取相关的 binding wrapper。
func RegisterBindings(reg *registry.Registry) {
	if reg == nil {
		RegisterBindingsCompat()
		return
	}
	registerBindings(reg)
}

// RegisterBindingsCompat 显式保留“写入全局 registry”的兼容 binding 注册入口。
func RegisterBindingsCompat() {
	registerBindings(registry.GlobalWithUsage("integrations/config.RegisterBindingsCompat"))
}

func registerBindings(reg *registry.Registry) {
	reg.RegisterBindings(runtime.NewBindingResolvers(
		registration("ConfigSection", coreintegration.MatchNamedWrapper(packagePath(), "Section"), resolveSection),
		registration("ConfigValue", coreintegration.MatchNamedWrapper(packagePath(), "Value"), resolveValue),
	)...)
}

func resolveSection(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	return resolveConfigWrapper(ctx, desc, true)
}

func resolveValue(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	return resolveConfigWrapper(ctx, desc, false)
}

func resolveConfigWrapper(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor, section bool) (any, error) {
	if ctx == nil || ctx.Container == nil {
		return nil, fmt.Errorf("config binding requires request container")
	}
	loader, err := coredi.GetByType[*Loader](ctx.Container)
	if err != nil {
		return nil, err
	}
	key := desc.BindingName
	holder := reflect.New(desc.InnerType)
	if section {
		if err := loader.Unmarshal(key, holder.Interface()); err != nil {
			return nil, err
		}
		return coreintegration.WrapFirstField(desc.WrapperType, holder.Elem())
	}
	value, ok := loader.Get(key)
	if !ok {
		return nil, fmt.Errorf("config key not found: %s", key)
	}
	rv := reflect.ValueOf(value)
	if rv.IsValid() && rv.Type().AssignableTo(desc.InnerType) {
		return coreintegration.WrapFirstField(desc.WrapperType, rv)
	}
	if err := loader.Unmarshal(key, holder.Interface()); err != nil {
		return nil, err
	}
	return coreintegration.WrapFirstField(desc.WrapperType, holder.Elem())
}

func registration(name string, match func(reflect.Type) (runtime.WrapperDescriptor, bool), resolve func(*runtime.HandlerContext, runtime.WrapperDescriptor) (any, error)) runtime.BindingRegistration {
	return runtime.BindingRegistration{
		Name:    name,
		Match:   match,
		Resolve: resolve,
	}
}

func packagePath() string {
	return reflect.TypeFor[Loader]().PkgPath()
}
