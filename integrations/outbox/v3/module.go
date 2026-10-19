// module.go 负责把 outbox integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package outbox

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

// build 创建 Outbox 集成模块，并注册服务、存储、分发器、配置与工厂。
func build(cfg Config) (module.Module, error) {
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
	base := module.New(module.Metadata{
		Providers: []*di.Provider{pService, pStore, pDispatcher, pConfigPtr, pConfigValue, pFactory},
		Exports:   []any{serviceToken, storeToken, dispatcherToken, configPtrToken, configValueToken, factoryToken},
	})
	return &integrationModule{BaseModule: base, service: service, config: cfg}, nil
}

// Init 初始化 Outbox 模块，并把 Outbox 相关 binding 注册到 registry。
// Module 创建 outbox 集成模块。
//
// 它是该集成唯一的装配入口：把模块加入根模块的 Imports 即可。
func Module(cfgs ...Config) module.Module {
	if len(cfgs) == 0 {
		return module.Failed(fmt.Errorf("outbox.Module requires at least one Config"))
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
func (m *integrationModule) Service() *Service { return m.service }

// Config 返回当前模块使用的 Outbox 配置。
func (m *integrationModule) Config() Config { return m.config }

