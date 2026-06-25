// module.go 负责把 logger integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package logger

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// Module 是 logger integration 对应的模块封装。
type Module struct {
	*module.BaseModule
	logger       Logger
	config       Config
	reg          *registry.Registry
	compatSource string
}

// ForRoot 创建日志集成模块，并把默认 logger、命名 logger、配置与工厂注册到容器中。
//
// 对业务代码来说，这是最常见的日志接入入口。
func ForRoot(cfg Config) (module.Module, error) {
	logger, err := New(cfg)
	if err != nil {
		return nil, err
	}
	cfg = normalizeConfig(cfg)

	loggerToken := reflect.TypeFor[Logger]()
	namedLoggerToken := LoggerToken(cfg.Name)
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()

	pLogger, err := di.ProviderFromInstanceWithToken(loggerToken, logger, di.Singleton)
	if err != nil {
		return nil, err
	}
	pNamedLogger, err := di.ProviderFromInstanceWithToken(namedLoggerToken, logger, di.Singleton)
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
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(logger.(interface {
		Default() Logger
		New(cfg Config) (Logger, error)
	})), di.Singleton)
	if err != nil {
		return nil, err
	}

	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pLogger, pNamedLogger, pConfigPtr, pConfigValue, pFactory},
		Exports:   []interface{}{loggerToken, namedLoggerToken, configPtrToken, configValueToken, factoryToken},
	})
	return &Module{BaseModule: base, logger: logger, config: cfg}, nil
}

// ForRootCompat 显式保留 standalone Init 写入全局 registry 的兼容语义。
func ForRootCompat(cfg Config) (module.Module, error) {
	logger, err := New(cfg)
	if err != nil {
		return nil, err
	}
	cfg = normalizeConfig(cfg)

	loggerToken := reflect.TypeFor[Logger]()
	namedLoggerToken := LoggerToken(cfg.Name)
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()

	pLogger, err := di.ProviderFromInstanceWithToken(loggerToken, logger, di.Singleton)
	if err != nil {
		return nil, err
	}
	pNamedLogger, err := di.ProviderFromInstanceWithToken(namedLoggerToken, logger, di.Singleton)
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
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(logger.(interface {
		Default() Logger
		New(cfg Config) (Logger, error)
	})), di.Singleton)
	if err != nil {
		return nil, err
	}

	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pLogger, pNamedLogger, pConfigPtr, pConfigValue, pFactory},
		Exports:   []interface{}{loggerToken, namedLoggerToken, configPtrToken, configValueToken, factoryToken},
	})
	return &Module{compatSource: "integrations/logger.ForRootCompat", BaseModule: base, logger: logger, config: cfg}, nil
}

// Init 初始化日志模块，并把日志相关绑定写入 registry。
func (m *Module) Init() error {
	if err := m.BaseModule.Init(); err != nil {
		return err
	}
	reg := m.reg
	if reg == nil {
		if m.compatSource == "" {
			return fmt.Errorf("logger.Module.Init requires an explicit registry; call SetRegistry(...) before Init or use logger.ForRootCompat(...) for standalone/global compatibility")
		}
		reg = registry.GlobalWithUsage(m.compatSource)
	}
	coreintegration.RegisterContainerBindings(reg,
		coreintegration.NewBindingEntry("Logger", reflect.TypeFor[Logger]()),
		coreintegration.NewBindingEntry("LoggerFactory", reflect.TypeFor[Factory]()),
		coreintegration.NewBindingEntry("LoggerConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("LoggerConfigPtr", reflect.TypeFor[*Config]()),
	)
	RegisterBindings(reg)
	return nil
}

// Logger 返回当前模块持有的默认 logger。
func (m *Module) Logger() Logger { return m.logger }

// Config 返回当前模块使用的日志配置。
func (m *Module) Config() Config { return m.config }

// SetRegistry 注入 registry，供 Init 阶段注册日志绑定。
func (m *Module) SetRegistry(reg *registry.Registry) { m.reg = reg }
