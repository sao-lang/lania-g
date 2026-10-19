// module.go 负责把 terminus integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package terminus

import (
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

// Config 描述 Terminus 健康检查模块的基础配置。
type Config struct {
	Version   string
	ReleaseID string
	// Path 是健康检查端点路径，默认 /health。
	Path string
}

// Factory 约定 Terminus 健康检查服务工厂的最小能力。
type Factory interface {
	Default() *HealthService
	New(cfg Config) (*HealthService, error)
}

type healthFactory struct {
	defaultService *HealthService
}

// Default 返回工厂持有的默认健康检查服务。
func (f *healthFactory) Default() *HealthService { return f.defaultService }

// New 根据给定配置创建一个新的健康检查服务。
func (f *healthFactory) New(cfg Config) (*HealthService, error) {
	svc := NewHealthService().SetVersion(cfg.Version).SetReleaseID(cfg.ReleaseID)
	return svc, nil
}

// Module 是 Terminus integration 对应的模块封装。
type integrationModule struct {
	*module.BaseModule
	service      *HealthService
	config       Config
}

// build 创建健康检查集成模块，并注册 HealthService、配置与工厂。
func build(cfg Config) (module.Module, error) {
	f := &healthFactory{}
	service, err := f.New(cfg)
	if err != nil {
		return nil, err
	}
	f.defaultService = service

	serviceToken := reflect.TypeFor[*HealthService]()
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()

	pService, err := di.ProviderFromInstanceWithToken(serviceToken, service, di.Singleton)
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
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(f), di.Singleton)
	if err != nil {
		return nil, err
	}

	base := module.New(module.Metadata{
		Providers: []*di.Provider{pService, pConfigPtr, pConfigValue, pFactory},
		Exports:   []interface{}{serviceToken, configPtrToken, configValueToken, factoryToken},
	})
	return &integrationModule{BaseModule: base, service: service, config: cfg}, nil
}

// Init 初始化 Terminus 模块，并把健康检查相关 binding 注册到 registry。
// Module 创建 terminus 集成模块。
//
// 它是该集成唯一的装配入口：把模块加入根模块的 Imports 即可。
func Module(cfg Config) module.Module {
	m, err := build(cfg)
	if err != nil {
		return module.Failed(err)
	}
	return m
}

func (m *integrationModule) Init() error {
	if err := m.BaseModule.Init(); err != nil {
		return err
	}
	reg := m.Context().Registry()
	coreintegration.RegisterContainerBindings(reg,
		coreintegration.NewBindingEntryFor[*HealthService]("TerminusHealthService"),
		coreintegration.NewBindingEntry("TerminusFactory", reflect.TypeFor[Factory]()),
		coreintegration.NewBindingEntry("TerminusConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("TerminusConfigPtr", reflect.TypeFor[*Config]()),
	)
	return nil
}

// Service 返回当前模块持有的默认健康检查服务。
func (m *integrationModule) Service() *HealthService { return m.service }

// Config 返回当前模块使用的健康检查配置。
func (m *integrationModule) Config() Config { return m.config }

