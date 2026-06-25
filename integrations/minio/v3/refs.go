// refs.go 定义 minio integration 的命名引用 wrapper 与 binding 注册辅助。
package minio

import (
	"fmt"
	"reflect"

	coredi "github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// ClientNamer 允许 marker 类型自定义 `ClientRef` 对应的 MinIO client 名称。
type ClientNamer interface {
	MinIOClientName() string
}

// DefaultClient 是默认 MinIO client 的 marker 类型。
type DefaultClient struct{}

// MinIOClientName 返回默认 client 名称。
func (DefaultClient) MinIOClientName() string { return "default" }

// InjectClient 用于在 handler 参数中注入默认 MinIO Client。
type InjectClient struct {
	*Client
}

// ClientRef 用于在 handler 参数中注入“按名字区分”的 MinIO Client。
type ClientRef[N any] struct {
	*Client
	_ *N
}

// ClientToken 返回按 name 区分的 MinIO client token。
func ClientToken(name string) string {
	if name == "" {
		name = "default"
	}
	return "minio:client:" + name
}

// RegisterBindings 注册 MinIO 相关的命名注入 wrapper。
func RegisterBindings(reg *registry.Registry) {
	if reg == nil {
		RegisterBindingsCompat()
		return
	}
	registerBindings(reg)
}

// RegisterBindingsCompat 显式保留“写入全局 registry”的兼容 binding 注册入口。
func RegisterBindingsCompat() {
	registerBindings(registry.GlobalWithUsage("integrations/minio.RegisterBindingsCompat"))
}

func registerBindings(reg *registry.Registry) {
	reg.RegisterBindings(runtime.NewBindingResolvers(
		registration("MinIOInjectClient", coreintegration.MatchNamedWrapper(packagePath(), "InjectClient"), resolveInjectClient),
		registration("MinIOClientRef", coreintegration.MatchNamedWrapper(packagePath(), "ClientRef"), resolveClientRef),
	)...)
}

func resolveInjectClient(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	client, err := resolveNamedClient(ctx, "default")
	if err != nil {
		return nil, err
	}
	return coreintegration.WrapFirstField(desc.WrapperType, reflect.ValueOf(client))
}

func resolveClientRef(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	client, err := resolveNamedClient(ctx, clientNameFromWrapper(desc.WrapperType))
	if err != nil {
		return nil, err
	}
	return coreintegration.WrapFirstField(desc.WrapperType, reflect.ValueOf(client))
}

func resolveNamedClient(ctx *runtime.HandlerContext, name string) (*Client, error) {
	if ctx == nil || ctx.Container == nil {
		return nil, fmt.Errorf("minio client binding requires request container")
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
		return namer.MinIOClientName(), true
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
	return reflect.TypeFor[ClientNamer]().PkgPath()
}
