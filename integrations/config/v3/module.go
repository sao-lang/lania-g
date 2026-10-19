// module.go 负责把 config integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package config

import (
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

// Module 是配置集成对应的模块封装。
type integrationModule struct {
	*module.BaseModule
	loader       *Loader
	config       Config
}

// build 创建配置集成模块，并把配置加载器与配置对象注册到 DI 容器中。
//
// 导出的常见依赖包括：
// - `*Loader`
// - `*Config`
// - `Config`
// - `Factory`
func build(cfg Config) (module.Module, error) {
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

	base := module.New(module.Metadata{
		Providers: []*di.Provider{pLoader, pConfigPtr, pConfigValue, pFactory},
		Exports:   []interface{}{loaderToken, configPtrToken, configValueToken, factoryToken},
	})
	return &integrationModule{BaseModule: base, loader: loader, config: cfg}, nil
}

// Init 初始化配置模块，并将配置相关绑定注册到 registry。
// 这样业务 handler 可以通过绑定包装类型按需读取配置值。
// Module 创建 config 集成模块。
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
		coreintegration.NewBindingEntryFor[*Loader]("ConfigLoader"),
		coreintegration.NewBindingEntry("ConfigLoaderFactory", reflect.TypeFor[Factory]()),
		coreintegration.NewBindingEntry("ConfigConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("ConfigConfigPtr", reflect.TypeFor[*Config]()),
	)
	registerBindings(reg)
	return nil
}

// Loader 返回当前模块持有的配置加载器。
func (m *integrationModule) Loader() *Loader { return m.loader }

// Config 返回当前模块的配置快照。
func (m *integrationModule) Config() Config { return m.config }

