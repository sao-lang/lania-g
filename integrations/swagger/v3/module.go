// module.go 负责把 swagger integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package swagger

import (
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

// Module 是 swagger integration 对应的模块封装。
type integrationModule struct {
	*module.BaseModule
	builder      *Builder
	config       Config
	uiConfig     *UIConfig
}

// ForRoot 创建 Swagger 集成模块，并注册文档构建器、配置、UI 配置与工厂。
func build(cfg Config, ui ...*UIConfig) (module.Module, error) {
	builder, err := New(cfg)
	if err != nil {
		return nil, err
	}
	uiConfig := DefaultUIConfig()
	if len(ui) > 0 && ui[0] != nil {
		uiConfig = ui[0]
	}

	builderToken := reflect.TypeFor[*Builder]()
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	uiConfigToken := reflect.TypeFor[*UIConfig]()
	factoryToken := reflect.TypeFor[Factory]()

	pBuilder, err := di.ProviderFromInstanceWithToken(builderToken, builder, di.Singleton)
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
	pUIConfig, err := di.ProviderFromInstanceWithToken(uiConfigToken, uiConfig, di.Singleton)
	if err != nil {
		return nil, err
	}
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(builder), di.Singleton)
	if err != nil {
		return nil, err
	}
	base := module.New(module.Metadata{
		Providers: []*di.Provider{pBuilder, pConfigPtr, pConfigValue, pUIConfig, pFactory},
		Exports:   []interface{}{builderToken, configPtrToken, configValueToken, uiConfigToken, factoryToken},
	})
	return &integrationModule{BaseModule: base, builder: builder, config: cfg, uiConfig: uiConfig}, nil
}

// Init 初始化 Swagger 模块，并把 Swagger 相关 binding 注册到 registry。
// Module 创建 swagger 集成模块。
//
// 它是该集成唯一的装配入口：把模块加入根模块的 Imports 即可。
func Module(cfg Config, ui ...*UIConfig) module.Module {
	m, err := build(cfg, ui...)
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
		coreintegration.NewBindingEntryFor[*Builder]("SwaggerBuilder"),
		coreintegration.NewBindingEntry("SwaggerFactory", reflect.TypeFor[Factory]()),
		coreintegration.NewBindingEntry("SwaggerConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("SwaggerConfigPtr", reflect.TypeFor[*Config]()),
		coreintegration.NewBindingEntry("SwaggerUIConfig", reflect.TypeFor[*UIConfig]()),
	)
	return nil
}

// Builder 返回当前模块持有的 Swagger 文档构建器。
func (m *integrationModule) Builder() *Builder { return m.builder }

// Config 返回当前模块使用的 Swagger 配置。
func (m *integrationModule) Config() Config { return m.config }

// UIConfig 返回当前模块使用的 Swagger UI 配置。
func (m *integrationModule) UIConfig() *UIConfig { return m.uiConfig }

