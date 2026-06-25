// binding.go 实现 kafka integration 暴露给 handler 的 binding 包装与 resolver。
package kafka

import (
	"reflect"

	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// RegisterBindings 注册 Kafka integration 的容器绑定与命名注入 wrapper。
func RegisterBindings(reg *registry.Registry) {
	if reg == nil {
		RegisterBindingsCompat()
		return
	}
	registerBindings(reg)
}

// RegisterBindingsCompat 显式保留“写入全局 registry”的兼容 binding 注册入口。
func RegisterBindingsCompat() {
	registerBindings(registry.GlobalWithUsage("integrations/kafka.RegisterBindingsCompat"))
}

func registerBindings(reg *registry.Registry) {
	coreintegration.RegisterContainerBindings(reg,
		coreintegration.NewBindingEntryFor[*Client]("KafkaClient"),
		coreintegration.NewBindingEntry("KafkaReaderFactory", reflect.TypeFor[ReaderFactory]()),
		coreintegration.NewBindingEntry("KafkaWriterFactory", reflect.TypeFor[WriterFactory]()),
		coreintegration.NewBindingEntry("KafkaConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("KafkaConfigPtr", reflect.TypeFor[*Config]()),
		coreintegration.NewBindingEntry("KafkaFactory", reflect.TypeFor[Factory]()),
	)
	RegisterNamedBindings(reg)
}
