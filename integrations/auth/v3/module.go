// module.go 负责把 auth integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package auth

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

// Module 是 auth integration 对应的模块封装。
type integrationModule struct {
	*module.BaseModule
	service      *Service
	config       Config
}

// ForRoot 创建认证集成模块，并注册默认服务、命名服务、配置与工厂。
func build(cfg Config) (module.Module, error) {
	service, err := New(cfg)
	if err != nil {
		return nil, err
	}
	cfg = service.Config()

	serviceToken := reflect.TypeFor[*Service]()
	namedServiceToken := ServiceToken(cfg.Name)
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()

	pService, err := di.ProviderFromInstanceWithToken(serviceToken, service, di.Singleton)
	if err != nil {
		return nil, err
	}
	pNamed, err := di.ProviderFromInstanceWithToken(namedServiceToken, service, di.Singleton)
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
		Providers: []*di.Provider{pService, pNamed, pConfigPtr, pConfigValue, pFactory},
		Exports:   []any{serviceToken, namedServiceToken, configPtrToken, configValueToken, factoryToken},
	})
	return &integrationModule{BaseModule: base, service: service, config: cfg}, nil
}

// Init 初始化认证模块，并把认证相关 binding 注册到 registry。
// Module 创建 auth 集成模块。
//
// 它是该集成唯一的装配入口：把模块加入根模块的 Imports 即可。
func Module(cfgs ...Config) module.Module {
	if len(cfgs) == 0 {
		return module.Failed(fmt.Errorf("auth.Module requires at least one Config"))
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
		coreintegration.NewBindingEntry("AuthService", reflect.TypeFor[*Service]()),
		coreintegration.NewBindingEntry("AuthConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("AuthConfigPtr", reflect.TypeFor[*Config]()),
		coreintegration.NewBindingEntry("AuthFactory", reflect.TypeFor[Factory]()),
	)
	registerBindings(reg)
	return nil
}

// Service 返回当前模块持有的认证服务。
func (m *integrationModule) Service() *Service { return m.service }

// Config 返回当前模块使用的认证配置。
func (m *integrationModule) Config() Config { return m.config }

// ServiceToken 返回命名认证服务对应的 DI token。
func ServiceToken(name string) string {
	if name == "" {
		name = DefaultName
	}
	return "auth:instance:" + name
}
