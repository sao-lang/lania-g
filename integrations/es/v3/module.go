// module.go 负责把 es integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package es

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// Module 是 Elasticsearch integration 的模块封装。
//
// 它会导出 `*Client`、`Config`、`Factory`，并在初始化阶段注册 ES 相关 binding。
type Module struct {
	*module.BaseModule
	client       *Client
	config       Config
	reg          *registry.Registry
	compatSource string
}

// ForRoot 基于配置创建一个 Elasticsearch 模块。
func ForRoot(cfg Config) (module.Module, error) {
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
	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pClient, pNamedClient, pConfigPtr, pConfigValue, pFactory},
		Exports:   []interface{}{clientToken, namedClientToken, configPtrToken, configValueToken, factoryToken},
	})
	return &Module{BaseModule: base, client: client, config: cfg}, nil
}

// ForRootCompat 显式保留 standalone Init 写入全局 registry 的兼容语义。
func ForRootCompat(cfg Config) (module.Module, error) {
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
	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pClient, pNamedClient, pConfigPtr, pConfigValue, pFactory},
		Exports:   []interface{}{clientToken, namedClientToken, configPtrToken, configValueToken, factoryToken},
	})
	return &Module{compatSource: "integrations/es.ForRootCompat", BaseModule: base, client: client, config: cfg}, nil
}

// Init 完成 BaseModule 初始化并注册 ES binding。
func (m *Module) Init() error {
	if err := m.BaseModule.Init(); err != nil {
		return err
	}
	reg := m.reg
	if reg == nil {
		if m.compatSource == "" {
			return fmt.Errorf("es.Module.Init requires an explicit registry; call SetRegistry(...) before Init or use es.ForRootCompat(...) for standalone/global compatibility")
		}
		reg = registry.GlobalWithUsage(m.compatSource)
	}
	coreintegration.RegisterContainerBindings(reg,
		coreintegration.NewBindingEntryFor[*Client]("ESClient"),
		coreintegration.NewBindingEntry("ESFactory", reflect.TypeFor[Factory]()),
		coreintegration.NewBindingEntry("ESConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("ESConfigPtr", reflect.TypeFor[*Config]()),
	)
	RegisterBindings(reg)
	return nil
}

// Client 返回模块内部持有的 ES Client。
func (m *Module) Client() *Client { return m.client }

// Config 返回模块配置。
func (m *Module) Config() Config { return m.config }

// SetRegistry 为 Init 阶段指定要写入的 registry；为空时使用全局 registry。
func (m *Module) SetRegistry(reg *registry.Registry) { m.reg = reg }
