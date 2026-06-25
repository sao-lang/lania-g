// refs.go 定义 http integration 的命名引用 wrapper 与 binding 注册辅助。
package http

import (
	"fmt"
	"reflect"

	coredi "github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// ClientNamer 约定命名 HTTP client 引用的名称来源。
type ClientNamer interface {
	HTTPClientName() string
}

// DefaultClient 用于标记“默认 HTTP client”引用。
type DefaultClient struct{}

// HTTPClientName 返回默认 HTTP client 的名称。
func (DefaultClient) HTTPClientName() string { return "default" }

// InjectClient 表示注入默认 HTTP client 的包装类型。
type InjectClient struct {
	*Client
}

// ClientRef 用于按命名引用注入一个 HTTP client。
// 泛型参数 N 通常是一个实现了 ClientNamer 的标记类型。
type ClientRef[N any] struct {
	*Client
	_ *N
}

// ClientToken 返回某个命名 HTTP client 对应的 DI token。
func ClientToken(name string) string {
	if name == "" {
		name = "default"
	}
	return "http:client:" + name
}

// RegisterBindings 注册 HTTP client 引用相关的 binding。
func RegisterBindings(reg *registry.Registry) {
	if reg == nil {
		RegisterBindingsCompat()
		return
	}
	registerBindings(reg)
}

// RegisterBindingsCompat 显式保留“写入全局 registry”的兼容 binding 注册入口。
func RegisterBindingsCompat() {
	registerBindings(registry.GlobalWithUsage("integrations/http.RegisterBindingsCompat"))
}

func registerBindings(reg *registry.Registry) {
	reg.RegisterBindings(runtime.NewBindingResolvers(
		registration("HTTPInjectClient", coreintegration.MatchNamedWrapper(packagePath(), "InjectClient"), resolveInjectClient),
		registration("HTTPClientRef", coreintegration.MatchNamedWrapper(packagePath(), "ClientRef"), resolveClientRef),
	)...)
}

func resolveInjectClient(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	client, err := resolveNamedClient(ctx, "default")
	if err != nil {
		return nil, err
	}
	return wrapClient(desc.WrapperType, reflect.ValueOf(client))
}

func resolveClientRef(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	client, err := resolveNamedClient(ctx, clientNameFromWrapper(desc.WrapperType))
	if err != nil {
		return nil, err
	}
	return wrapClient(desc.WrapperType, reflect.ValueOf(client))
}

func resolveNamedClient(ctx *runtime.HandlerContext, name string) (*Client, error) {
	if ctx == nil || ctx.Container == nil {
		return nil, fmt.Errorf("http client binding requires request container")
	}
	if value, err := ctx.Container.Get(ClientToken(name)); err == nil {
		if client, ok := value.(*Client); ok {
			return client, nil
		}
	}
	value, err := coredi.GetByType[*Client](ctx.Container)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func clientNameFromWrapper(wrapperType reflect.Type) string {
	return coreintegration.ResolveMarkerName(wrapperType, "default", func(marker any) (string, bool) {
		namer, ok := marker.(ClientNamer)
		if !ok {
			return "", false
		}
		return namer.HTTPClientName(), true
	})
}

func wrapClient(wrapperType reflect.Type, value reflect.Value) (any, error) {
	wrapped, err := coreintegration.WrapFirstField(wrapperType, value)
	if err == nil {
		return wrapped, nil
	}
	if wrapperType.Kind() == reflect.Ptr {
		wrapperType = wrapperType.Elem()
	}
	return nil, fmt.Errorf("invalid http wrapper: %s", wrapperType.String())
}

func registration(name string, match func(reflect.Type) (runtime.WrapperDescriptor, bool), resolve func(*runtime.HandlerContext, runtime.WrapperDescriptor) (any, error)) runtime.BindingRegistration {
	return runtime.BindingRegistration{
		Name:    name,
		Match:   match,
		Resolve: resolve,
	}
}

func packagePath() string {
	return reflect.TypeFor[ClientNamer]().PkgPath()
}
