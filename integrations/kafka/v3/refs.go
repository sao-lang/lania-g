// refs.go 定义 kafka integration 的命名引用 wrapper 与 binding 注册辅助。
package kafka

import (
	"fmt"
	"reflect"

	kgo "github.com/segmentio/kafka-go"
	coredi "github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// ClientNamer 允许 marker 类型自定义 `ClientRef` 对应的 Kafka client 名称。
type ClientNamer interface {
	KafkaClientName() string
}

// DefaultClient 是默认 Kafka client 的 marker 类型。
type DefaultClient struct{}

// KafkaClientName 返回默认 client 名称。
func (DefaultClient) KafkaClientName() string { return "default" }

// InjectClient 用于在 handler 参数中注入默认 Kafka Client。
type InjectClient struct {
	*Client
}

// ClientRef 用于在 handler 参数中注入“按名字区分”的 Kafka Client。
type ClientRef[N any] struct {
	*Client
	_ *N
}

// InjectReaderFactory 用于注入 ReaderFactory。
type InjectReaderFactory struct {
	ReaderFactory
}

// InjectWriterFactory 用于注入 WriterFactory。
type InjectWriterFactory struct {
	WriterFactory
}

// ClientToken 返回按 name 区分的 Kafka client token。
func ClientToken(name string) string {
	if name == "" {
		name = "default"
	}
	return "kafka:client:" + name
}

// RegisterNamedBindings 注册 Kafka 相关的命名注入 wrapper。
func RegisterNamedBindings(reg *registry.Registry) {
	if reg == nil {
		RegisterNamedBindingsCompat()
		return
	}
	registerNamedBindings(reg)
}

// RegisterNamedBindingsCompat 显式保留“写入全局 registry”的兼容命名 binding 注册入口。
func RegisterNamedBindingsCompat() {
	registerNamedBindings(registry.GlobalWithUsage("integrations/kafka.RegisterNamedBindingsCompat"))
}

func registerNamedBindings(reg *registry.Registry) {
	reg.RegisterBindings(runtime.NewBindingResolvers(
		registration("KafkaInjectClient", coreintegration.MatchNamedWrapper(packagePath(), "InjectClient"), resolveInjectClient),
		registration("KafkaClientRef", coreintegration.MatchNamedWrapper(packagePath(), "ClientRef"), resolveClientRef),
		registration("KafkaInjectReaderFactory", coreintegration.MatchNamedWrapper(packagePath(), "InjectReaderFactory"), resolveReaderFactory),
		registration("KafkaInjectWriterFactory", coreintegration.MatchNamedWrapper(packagePath(), "InjectWriterFactory"), resolveWriterFactory),
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

func resolveReaderFactory(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	value, err := coredi.GetByType[ReaderFactory](ctx.Container)
	if err != nil {
		return nil, err
	}
	return coreintegration.WrapFirstField(desc.WrapperType, reflect.ValueOf(value))
}

func resolveWriterFactory(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	value, err := coredi.GetByType[WriterFactory](ctx.Container)
	if err != nil {
		return nil, err
	}
	return coreintegration.WrapFirstField(desc.WrapperType, reflect.ValueOf(value))
}

func resolveNamedClient(ctx *runtime.HandlerContext, name string) (*Client, error) {
	if ctx == nil || ctx.Container == nil {
		return nil, fmt.Errorf("kafka client binding requires request container")
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
		return namer.KafkaClientName(), true
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

var _ = kgo.RequireAll
