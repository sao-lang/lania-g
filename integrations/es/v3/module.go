// module.go 负责把 es integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package es

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

// Module 是 Elasticsearch integration 的模块封装。
//
// 它会导出 `*Client`、`Config`、`Factory`，并在初始化阶段注册 ES 相关 binding。
type integrationModule struct {
	*module.BaseModule
	client       *Client
	config       Config
}

// ForRoot 基于配置创建一个 Elasticsearch 模块。
func build(cfg Config) (module.Module, error) {
	client, err := New(cfg)
	if err != nil {
		return nil, err
	}
	cfg = client.Config()

	clientToken := reflect.TypeFor[*Client]()
	namedClientToken := ClientToken(cfg.Name)
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()

	pClient, err := di.ProviderFromInstanceWithToken(clientToken, client, di.Singleton)
	if err != nil {
		return nil, err
	}
	pNamedClient, err := di.ProviderFromInstanceWithToken(namedClientToken, client, di.Singleton)
	if err != nil {
		return nil, err
	}
	pConfigPtr, err := di.ProviderFromInstanceWithToken(configPtrToken, &cfg, di.Singleton)
	if err != nil {
		return nil, err
	}
	pConfigValue, err := di.ProviderFromInstanceWithToken(configValueToken, cfg, di.Singleton)
	if err != nil {
		return nil, err
	}
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(client), di.Singleton)
	if err != nil {
		return nil, err
	}
	base := module.New(module.Metadata{
		Providers: []*di.Provider{pClient, pNamedClient, pConfigPtr, pConfigValue, pFactory},
		Exports:   []interface{}{clientToken, namedClientToken, configPtrToken, configValueToken, factoryToken},
	})
	return &integrationModule{BaseModule: base, client: client, config: cfg}, nil
}

// Init 完成 BaseModule 初始化并注册 ES binding。
// Module 创建 es 集成模块。
//
// 它是该集成唯一的装配入口：把模块加入根模块的 Imports 即可。
func Module(cfgs ...Config) module.Module {
	if len(cfgs) == 0 {
		return module.Failed(fmt.Errorf("es.Module requires at least one Config"))
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

func (m *integrationModule) Init() error {
	if err := m.BaseModule.Init(); err != nil {
		return err
	}
	reg := m.Context().Registry()
	coreintegration.RegisterContainerBindings(reg,
		coreintegration.NewBindingEntryFor[*Client]("ESClient"),
		coreintegration.NewBindingEntry("ESFactory", reflect.TypeFor[Factory]()),
		coreintegration.NewBindingEntry("ESConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("ESConfigPtr", reflect.TypeFor[*Config]()),
	)
	registerBindings(reg)
	return nil
}

// Client 返回模块内部持有的 ES Client。
func (m *integrationModule) Client() *Client { return m.client }

// Config 返回模块配置。
func (m *integrationModule) Config() Config { return m.config }

