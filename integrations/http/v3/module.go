// module.go 负责把 http integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package http

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// Module 是 HTTP integration 对应的模块封装。
type Module struct {
	*module.BaseModule
	client       *Client
	config       Config
	reg          *registry.Registry
	compatSource string
}

// ForRoot 创建 HTTP 客户端集成模块，并把默认 client、命名 client、配置与工厂注册到容器中。
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
	return &Module{compatSource: "integrations/http.ForRootCompat", BaseModule: base, client: client, config: cfg}, nil
}

// Init 初始化 HTTP 集成模块，并把 HTTP 相关 binding 注册到 registry。
func (m *Module) Init() error {
	if err := m.BaseModule.Init(); err != nil {
		return err
	}
	reg := m.reg
	if reg == nil {
		if m.compatSource == "" {
			return fmt.Errorf("http.Module.Init requires an explicit registry; call SetRegistry(...) before Init or use http.ForRootCompat(...) for standalone/global compatibility")
		}
		reg = registry.GlobalWithUsage(m.compatSource)
	}
	coreintegration.RegisterContainerBindings(reg,
		coreintegration.NewBindingEntryFor[*Client]("HTTPClient"),
		coreintegration.NewBindingEntry("HTTPClientFactory", reflect.TypeFor[Factory]()),
		coreintegration.NewBindingEntry("HTTPClientConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("HTTPClientConfigPtr", reflect.TypeFor[*Config]()),
	)
	RegisterBindings(reg)
	return nil
}

// Client 返回当前模块持有的默认 HTTP client。
func (m *Module) Client() *Client { return m.client }

// Config 返回当前模块使用的 HTTP client 配置快照。
func (m *Module) Config() Config { return cloneConfig(m.config) }

// SetRegistry 注入 registry，供 Init 阶段注册绑定声明。
func (m *Module) SetRegistry(reg *registry.Registry) { m.reg = reg }
