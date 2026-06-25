// module.go 负责把 swagger integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package swagger

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// Module 是 swagger integration 对应的模块封装。
type Module struct {
	*module.BaseModule
	builder      *Builder
	config       Config
	uiConfig     *UIConfig
	reg          *registry.Registry
	compatSource string
}

// ForRoot 创建 Swagger 集成模块，并注册文档构建器、配置、UI 配置与工厂。
func ForRoot(cfg Config, ui ...*UIConfig) (module.Module, error) {
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
	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pBuilder, pConfigPtr, pConfigValue, pUIConfig, pFactory},
		Exports:   []interface{}{builderToken, configPtrToken, configValueToken, uiConfigToken, factoryToken},
	})
	return &Module{BaseModule: base, builder: builder, config: cfg, uiConfig: uiConfig}, nil
}

// ForRootCompat 显式保留 standalone Init 写入全局 registry 的兼容语义。
func ForRootCompat(cfg Config, ui ...*UIConfig) (module.Module, error) {
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
	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pBuilder, pConfigPtr, pConfigValue, pUIConfig, pFactory},
		Exports:   []interface{}{builderToken, configPtrToken, configValueToken, uiConfigToken, factoryToken},
	})
	return &Module{compatSource: "integrations/swagger.ForRootCompat", BaseModule: base, builder: builder, config: cfg, uiConfig: uiConfig}, nil
}

// Init 初始化 Swagger 模块，并把 Swagger 相关 binding 注册到 registry。
func (m *Module) Init() error {
	if err := m.BaseModule.Init(); err != nil {
		return err
	}
	reg := m.reg
	if reg == nil {
		if m.compatSource == "" {
			return fmt.Errorf("swagger.Module.Init requires an explicit registry; call SetRegistry(...) before Init or use swagger.ForRootCompat(...) for standalone/global compatibility")
		}
		reg = registry.GlobalWithUsage(m.compatSource)
	}
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
func (m *Module) Builder() *Builder { return m.builder }

// Config 返回当前模块使用的 Swagger 配置。
func (m *Module) Config() Config { return m.config }

// UIConfig 返回当前模块使用的 Swagger UI 配置。
func (m *Module) UIConfig() *UIConfig { return m.uiConfig }

// SetRegistry 注入 registry，供 Init 阶段注册绑定声明。
func (m *Module) SetRegistry(reg *registry.Registry) { m.reg = reg }
