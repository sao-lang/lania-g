// module.go 负责把 logger integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package logger

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

// Module 是 logger integration 对应的模块封装。
type integrationModule struct {
	*module.BaseModule
	logger       Logger
	config       Config
}

// build 创建日志集成模块，并把默认 logger、命名 logger、配置与工厂注册到容器中。
//
// 对业务代码来说，这是最常见的日志接入入口。
func build(cfg Config) (module.Module, error) {
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

	base := module.New(module.Metadata{
		Providers: []*di.Provider{pLogger, pNamedLogger, pConfigPtr, pConfigValue, pFactory},
		Exports:   []interface{}{loggerToken, namedLoggerToken, configPtrToken, configValueToken, factoryToken},
	})
	return &integrationModule{BaseModule: base, logger: logger, config: cfg}, nil
}

// Init 初始化日志模块，并把日志相关绑定写入 registry。
// Module 创建 logger 集成模块。
//
// 它是该集成唯一的装配入口：把模块加入根模块的 Imports 即可。
func Module(cfgs ...Config) module.Module {
	if len(cfgs) == 0 {
		return module.Failed(fmt.Errorf("logger.Module requires at least one Config"))
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
		coreintegration.NewBindingEntry("Logger", reflect.TypeFor[Logger]()),
		coreintegration.NewBindingEntry("LoggerFactory", reflect.TypeFor[Factory]()),
		coreintegration.NewBindingEntry("LoggerConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("LoggerConfigPtr", reflect.TypeFor[*Config]()),
	)
	registerBindings(reg)
	return nil
}

// Logger 返回当前模块持有的默认 logger。
func (m *integrationModule) Logger() Logger { return m.logger }

// Config 返回当前模块使用的日志配置。
func (m *integrationModule) Config() Config { return m.config }

