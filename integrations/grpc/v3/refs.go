// refs.go 定义 grpc integration 的命名引用 wrapper 与 binding 注册辅助。
package grpc

import (
	"fmt"
	"reflect"

	gogrpc "google.golang.org/grpc"
	coredi "github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// ClientNamer 允许 marker 类型自定义要注入的 gRPC client 名称。
type ClientNamer interface {
	GRPCClientName() string
}

// DefaultClient 用于标记“默认 gRPC client”引用。
type DefaultClient struct{}

// GRPCClientName 返回默认 gRPC client 名称。
func (DefaultClient) GRPCClientName() string { return "default" }

// InjectClient 用于直接注入默认 gRPC Client。
type InjectClient struct {
	*Client
}

// ClientRef 用于按命名引用注入一个 gRPC client。
type ClientRef[N any] struct {
	*Client
	_ *N
}

// InjectConn 用于直接注入默认 gRPC 连接。
type InjectConn struct {
	*gogrpc.ClientConn
}

// ConnRef 用于按命名引用注入一个 gRPC 连接。
type ConnRef[N any] struct {
	*gogrpc.ClientConn
	_ *N
}

// ClientToken 返回某个命名 gRPC client 对应的 DI token。
func ClientToken(name string) string {
	if name == "" {
		name = "default"
	}
	return "grpc:client:" + name
}

// ConnToken 返回某个命名 gRPC 连接对应的 DI token。
func ConnToken(name string) string {
	if name == "" {
		name = "default"
	}
	return "grpc:conn:" + name
}

// RegisterBindings 注册 gRPC client/conn 引用相关的 binding。
func RegisterBindings(reg *registry.Registry) {
	if reg == nil {
		RegisterBindingsCompat()
		return
	}
	registerBindings(reg)
}

// RegisterBindingsCompat 显式保留“写入全局 registry”的兼容 binding 注册入口。
func RegisterBindingsCompat() {
	registerBindings(registry.GlobalWithUsage("integrations/grpc.RegisterBindingsCompat"))
}

func registerBindings(reg *registry.Registry) {
	reg.RegisterBindings(runtime.NewBindingResolvers(
		registration("GRPCInjectClient", coreintegration.MatchNamedWrapper(packagePath(), "InjectClient"), resolveInjectClient),
		registration("GRPCClientRef", coreintegration.MatchNamedWrapper(packagePath(), "ClientRef"), resolveClientRef),
		registration("GRPCInjectConn", coreintegration.MatchNamedWrapper(packagePath(), "InjectConn"), resolveInjectConn),
		registration("GRPCConnRef", coreintegration.MatchNamedWrapper(packagePath(), "ConnRef"), resolveConnRef),
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

func resolveInjectConn(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	conn, err := resolveNamedConn(ctx, "default")
	if err != nil {
		return nil, err
	}
	return coreintegration.WrapFirstField(desc.WrapperType, reflect.ValueOf(conn))
}

func resolveConnRef(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	conn, err := resolveNamedConn(ctx, clientNameFromWrapper(desc.WrapperType))
	if err != nil {
		return nil, err
	}
	return coreintegration.WrapFirstField(desc.WrapperType, reflect.ValueOf(conn))
}

func resolveNamedClient(ctx *runtime.HandlerContext, name string) (*Client, error) {
	if ctx == nil || ctx.Container == nil {
		return nil, fmt.Errorf("grpc client binding requires request container")
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

func resolveNamedConn(ctx *runtime.HandlerContext, name string) (*gogrpc.ClientConn, error) {
	if ctx == nil || ctx.Container == nil {
		return nil, fmt.Errorf("grpc conn binding requires request container")
	}
	if value, err := ctx.Container.Get(ConnToken(name)); err == nil {
		if conn, ok := value.(*gogrpc.ClientConn); ok {
			return conn, nil
		}
	}
	value, err := coredi.GetByType[*gogrpc.ClientConn](ctx.Container)
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
		return namer.GRPCClientName(), true
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
