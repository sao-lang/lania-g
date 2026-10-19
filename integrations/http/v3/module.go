// module.go 负责把 http integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package http

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

// Module 是 HTTP integration 对应的模块封装。
type integrationModule struct {
	*module.BaseModule
	client       *Client
	config       Config
}

// ForRoot 创建 HTTP 客户端集成模块，并把默认 client、命名 client、配置与工厂注册到容器中。
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

// Init 初始化 HTTP 集成模块，并把 HTTP 相关 binding 注册到 registry。
// Module 创建 http 集成模块。
//
// 它是该集成唯一的装配入口：把模块加入根模块的 Imports 即可。
func Module(cfgs ...Config) module.Module {
	if len(cfgs) == 0 {
		return module.Failed(fmt.Errorf("http.Module requires at least one Config"))
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
		coreintegration.NewBindingEntryFor[*Client]("HTTPClient"),
		coreintegration.NewBindingEntry("HTTPClientFactory", reflect.TypeFor[Factory]()),
		coreintegration.NewBindingEntry("HTTPClientConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("HTTPClientConfigPtr", reflect.TypeFor[*Config]()),
	)
	registerBindings(reg)
	return nil
}

// Client 返回当前模块持有的默认 HTTP client。
func (m *integrationModule) Client() *Client { return m.client }

// Config 返回当前模块使用的 HTTP client 配置快照。
func (m *integrationModule) Config() Config { return cloneConfig(m.config) }

