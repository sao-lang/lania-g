// module.go 负责把 terminus integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package terminus

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// Config 描述 Terminus 健康检查模块的基础配置。
type Config struct {
	Version   string
	ReleaseID string
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
type Module struct {
	*module.BaseModule
	service      *HealthService
	config       Config
	reg          *registry.Registry
	compatSource string
}

// ForRoot 创建健康检查集成模块，并注册 HealthService、配置与工厂。
func ForRoot(cfg Config) (module.Module, error) {
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

	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pService, pConfigPtr, pConfigValue, pFactory},
		Exports:   []interface{}{serviceToken, configPtrToken, configValueToken, factoryToken},
	})
	return &Module{BaseModule: base, service: service, config: cfg}, nil
}

// ForRootCompat 显式保留 standalone Init 写入全局 registry 的兼容语义。
func ForRootCompat(cfg Config) (module.Module, error) {
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

	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pService, pConfigPtr, pConfigValue, pFactory},
		Exports:   []interface{}{serviceToken, configPtrToken, configValueToken, factoryToken},
	})
	return &Module{compatSource: "integrations/terminus.ForRootCompat", BaseModule: base, service: service, config: cfg}, nil
}

// Init 初始化 Terminus 模块，并把健康检查相关 binding 注册到 registry。
func (m *Module) Init() error {
	if err := m.BaseModule.Init(); err != nil {
		return err
	}
	reg := m.reg
	if reg == nil {
		if m.compatSource == "" {
			return fmt.Errorf("terminus.Module.Init requires an explicit registry; call SetRegistry(...) before Init or use terminus.ForRootCompat(...) for standalone/global compatibility")
		}
		reg = registry.GlobalWithUsage(m.compatSource)
	}
	coreintegration.RegisterContainerBindings(reg,
		coreintegration.NewBindingEntryFor[*HealthService]("TerminusHealthService"),
		coreintegration.NewBindingEntry("TerminusFactory", reflect.TypeFor[Factory]()),
		coreintegration.NewBindingEntry("TerminusConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("TerminusConfigPtr", reflect.TypeFor[*Config]()),
	)
	return nil
}

// Service 返回当前模块持有的默认健康检查服务。
func (m *Module) Service() *HealthService { return m.service }

// Config 返回当前模块使用的健康检查配置。
func (m *Module) Config() Config { return m.config }

// SetRegistry 注入 registry，供 Init 阶段注册绑定声明。
func (m *Module) SetRegistry(reg *registry.Registry) { m.reg = reg }
