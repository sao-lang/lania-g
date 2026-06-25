// module.go 负责把 config integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package config

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// Module 是配置集成对应的模块封装。
type Module struct {
	*module.BaseModule
	loader       *Loader
	config       Config
	reg          *registry.Registry
	compatSource string
}

// ForRoot 创建配置集成模块，并把配置加载器与配置对象注册到 DI 容器中。
//
// 导出的常见依赖包括：
// - `*Loader`
// - `*Config`
// - `Config`
// - `Factory`
func ForRoot(cfg Config) (module.Module, error) {
	loader, err := NewLoader(cfg)
	if err != nil {
		return nil, err
	}

	loaderToken := reflect.TypeFor[*Loader]()
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()

	pLoader, err := di.ProviderFromInstanceWithToken(loaderToken, loader, di.Singleton)
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
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(loader), di.Singleton)
	if err != nil {
		return nil, err
	}

	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pLoader, pConfigPtr, pConfigValue, pFactory},
		Exports:   []interface{}{loaderToken, configPtrToken, configValueToken, factoryToken},
	})
	return &Module{BaseModule: base, loader: loader, config: cfg}, nil
}

// ForRootCompat 显式保留 standalone Init 写入全局 registry 的兼容语义。
func ForRootCompat(cfg Config) (module.Module, error) {
	loader, err := NewLoader(cfg)
	if err != nil {
		return nil, err
	}

	loaderToken := reflect.TypeFor[*Loader]()
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()

	pLoader, err := di.ProviderFromInstanceWithToken(loaderToken, loader, di.Singleton)
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
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(loader), di.Singleton)
	if err != nil {
		return nil, err
	}

	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pLoader, pConfigPtr, pConfigValue, pFactory},
		Exports:   []interface{}{loaderToken, configPtrToken, configValueToken, factoryToken},
	})
	return &Module{compatSource: "integrations/config.ForRootCompat", BaseModule: base, loader: loader, config: cfg}, nil
}

// Init 初始化配置模块，并将配置相关绑定注册到 registry。
// 这样业务 handler 可以通过绑定包装类型按需读取配置值。
func (m *Module) Init() error {
	if err := m.BaseModule.Init(); err != nil {
		return err
	}
	reg := m.reg
	if reg == nil {
		if m.compatSource == "" {
			return fmt.Errorf("config.Module.Init requires an explicit registry; call SetRegistry(...) before Init or use config.ForRootCompat(...) for standalone/global compatibility")
		}
		reg = registry.GlobalWithUsage(m.compatSource)
	}
	coreintegration.RegisterContainerBindings(reg,
		coreintegration.NewBindingEntryFor[*Loader]("ConfigLoader"),
		coreintegration.NewBindingEntry("ConfigLoaderFactory", reflect.TypeFor[Factory]()),
		coreintegration.NewBindingEntry("ConfigConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("ConfigConfigPtr", reflect.TypeFor[*Config]()),
	)
	RegisterBindings(reg)
	return nil
}

// Loader 返回当前模块持有的配置加载器。
func (m *Module) Loader() *Loader { return m.loader }

// Config 返回当前模块的配置快照。
func (m *Module) Config() Config { return m.config }

// SetRegistry 注入 registry，供 Init 阶段写入绑定声明。
func (m *Module) SetRegistry(reg *registry.Registry) { m.reg = reg }
