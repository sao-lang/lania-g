// module.go 负责把 kafka integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package kafka

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

// KafkaModule 是 Kafka integration 的模块封装。
//
// 它会导出 `*Client`、`Config`、`Factory` 以及 reader/writer factory，
// 并在初始化时注册 Kafka 相关 binding。
type KafkaModule struct {
	*module.BaseModule
	client       *Client
	config       *Config
}

// ForRoot 基于配置创建一个 Kafka 模块。
func build(cfg Config) (module.Module, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}

	cfgCopy := client.Config()

	clientToken := reflect.TypeFor[*Client]()
	namedClientToken := ClientToken(cfgCopy.Name)
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	readerFactoryToken := reflect.TypeFor[ReaderFactory]()
	writerFactoryToken := reflect.TypeFor[WriterFactory]()
	factoryToken := reflect.TypeFor[Factory]()

	pClient, err := di.ProviderFromInstanceWithToken(clientToken, client, di.Singleton)
	if err != nil {
		return nil, err
	}
	pNamedClient, err := di.ProviderFromInstanceWithToken(namedClientToken, client, di.Singleton)
	if err != nil {
		return nil, err
	}
	pConfigPtr, err := di.ProviderFromInstanceWithToken(configPtrToken, &cfgCopy, di.Singleton)
	if err != nil {
		return nil, err
	}
	pConfigValue, err := di.ProviderFromInstanceWithToken(configValueToken, cfgCopy, di.Singleton)
	if err != nil {
		return nil, err
	}
	pReaderFactory, err := di.ProviderFromInstanceWithToken(readerFactoryToken, ReaderFactory(client), di.Singleton)
	if err != nil {
		return nil, err
	}
	pWriterFactory, err := di.ProviderFromInstanceWithToken(writerFactoryToken, WriterFactory(client), di.Singleton)
	if err != nil {
		return nil, err
	}
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(client), di.Singleton)
	if err != nil {
		return nil, err
	}

	base := module.New(module.Metadata{
		Imports:   nil,
		Providers: []*di.Provider{pClient, pNamedClient, pConfigPtr, pConfigValue, pReaderFactory, pWriterFactory, pFactory},
		Exports:   []interface{}{clientToken, namedClientToken, configPtrToken, configValueToken, readerFactoryToken, writerFactoryToken, factoryToken},
	})
	return &KafkaModule{
		BaseModule: base,
		client:     client,
		config:     &cfgCopy,
	}, nil
}

// Init 完成 BaseModule 初始化并注册 Kafka binding。
func (m *KafkaModule) Init() error {
	if err := m.BaseModule.Init(); err != nil {
		return err
	}
	reg := m.Context().Registry()
	registerBindings(reg)
	return nil
}

// Client 返回模块内部持有的 Kafka Client。
func (m *KafkaModule) Client() *Client { return m.client }

// Config 返回模块配置副本。
func (m *KafkaModule) Config() Config {
	if m == nil || m.config == nil {
		return Config{}
	}
	return m.config.clone()
}

// Module 创建 kafka 集成模块。
//
// 它是该集成唯一的装配入口：把模块加入根模块的 Imports 即可。
func Module(cfgs ...Config) module.Module {
	if len(cfgs) == 0 {
		return module.Failed(fmt.Errorf("kafka.Module requires at least one Config"))
	}
	modules := make([]module.Module, 0, len(cfgs))
	for _, cfg := range cfgs {
		m, err := build(cfg)
		if err != nil {
			return module.Failed(err)
		}
		modules = append(modules, m)
	}
	if len(modules) == 1 {
		return modules[0]
	}
	return module.New(module.Metadata{Imports: modules})
}

