// module.go 负责把 outbox integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package outbox

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// ForRoot 创建 Outbox 集成模块，并注册服务、存储、分发器、配置与工厂。
func ForRoot(cfg Config) (module.Module, error) {
	service, err := New(cfg)
	if err != nil {
		return nil, err
	}
	cfg = service.Config()

	serviceToken := reflect.TypeFor[*Service]()
	storeToken := reflect.TypeFor[Store]()
	dispatcherToken := reflect.TypeFor[Dispatcher]()
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()

	pService, err := di.ProviderFromInstanceWithToken(serviceToken, service, di.Singleton)
	if err != nil {
		return nil, err
	}
	pStore, err := di.ProviderFromInstanceWithToken(storeToken, cfg.Store, di.Singleton)
	if err != nil {
		return nil, err
	}
	pDispatcher, err := di.ProviderFromInstanceWithToken(dispatcherToken, cfg.Dispatcher, di.Singleton)
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
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(service), di.Singleton)
	if err != nil {
		return nil, err
	}
	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pService, pStore, pDispatcher, pConfigPtr, pConfigValue, pFactory},
		Exports:   []any{serviceToken, storeToken, dispatcherToken, configPtrToken, configValueToken, factoryToken},
	})
	return &Module{BaseModule: base, service: service, config: cfg}, nil
}

// ForRootCompat 显式保留 standalone Init 写入全局 registry 的兼容语义。
func ForRootCompat(cfg Config) (module.Module, error) {
	service, err := New(cfg)
	if err != nil {
		return nil, err
	}
	cfg = service.Config()

	serviceToken := reflect.TypeFor[*Service]()
	storeToken := reflect.TypeFor[Store]()
	dispatcherToken := reflect.TypeFor[Dispatcher]()
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()

	pService, err := di.ProviderFromInstanceWithToken(serviceToken, service, di.Singleton)
	if err != nil {
		return nil, err
	}
	pStore, err := di.ProviderFromInstanceWithToken(storeToken, cfg.Store, di.Singleton)
	if err != nil {
		return nil, err
	}
	pDispatcher, err := di.ProviderFromInstanceWithToken(dispatcherToken, cfg.Dispatcher, di.Singleton)
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
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(service), di.Singleton)
	if err != nil {
		return nil, err
	}
	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pService, pStore, pDispatcher, pConfigPtr, pConfigValue, pFactory},
		Exports:   []any{serviceToken, storeToken, dispatcherToken, configPtrToken, configValueToken, factoryToken},
	})
	return &Module{compatSource: "integrations/outbox.ForRootCompat", BaseModule: base, service: service, config: cfg}, nil
}

// Init 初始化 Outbox 模块，并把 Outbox 相关 binding 注册到 registry。
func (m *Module) Init() error {
	if err := m.BaseModule.Init(); err != nil {
		return err
	}
	reg := m.reg
	if reg == nil {
		if m.compatSource == "" {
			return fmt.Errorf("outbox.Module.Init requires an explicit registry; call SetRegistry(...) before Init or use outbox.ForRootCompat(...) for standalone/global compatibility")
		}
		reg = registry.GlobalWithUsage(m.compatSource)
	}
	coreintegration.RegisterContainerBindings(reg,
		coreintegration.NewBindingEntry("OutboxService", reflect.TypeFor[*Service]()),
		coreintegration.NewBindingEntry("OutboxStore", reflect.TypeFor[Store]()),
		coreintegration.NewBindingEntry("OutboxDispatcher", reflect.TypeFor[Dispatcher]()),
		coreintegration.NewBindingEntry("OutboxConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("OutboxConfigPtr", reflect.TypeFor[*Config]()),
		coreintegration.NewBindingEntry("OutboxFactory", reflect.TypeFor[Factory]()),
	)
	return nil
}

// Service 返回当前模块持有的 Outbox 服务。
func (m *Module) Service() *Service { return m.service }

// Config 返回当前模块使用的 Outbox 配置。
func (m *Module) Config() Config { return m.config }

// SetRegistry 注入 registry，供 Init 阶段注册绑定声明。
func (m *Module) SetRegistry(reg *registry.Registry) { m.reg = reg }
