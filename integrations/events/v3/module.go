// module.go 负责把 events integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package events

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// Module 是事件集成对应的模块封装。
type Module struct {
	*module.BaseModule
	bus          *Bus
	config       Config
	reg          *registry.Registry
	compatSource string
}

// ForRoot 创建事件集成模块，并注册事件总线、发射器、配置和工厂。
func ForRoot(cfg Config) (module.Module, error) {
	bus, err := New(cfg)
	if err != nil {
		return nil, err
	}
	busToken := reflect.TypeFor[*Bus]()
	emitterToken := reflect.TypeFor[Emitter]()
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()
	lifecycleToken := reflect.TypeFor[interface{ OnApplicationBootstrap() error }]()

	pBus, err := di.ProviderFromInstanceWithToken(busToken, bus, di.Singleton)
	if err != nil {
		return nil, err
	}
	pEmitter, err := di.ProviderFromInstanceWithToken(emitterToken, Emitter(bus), di.Singleton)
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
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(bus), di.Singleton)
	if err != nil {
		return nil, err
	}
	pLifecycle, err := di.ProviderFromInstanceWithToken(lifecycleToken, NewLifecycleHook(bus, nil), di.Singleton)
	if err != nil {
		return nil, err
	}
	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pBus, pEmitter, pConfigPtr, pConfigValue, pFactory, pLifecycle},
		Exports:   []interface{}{busToken, emitterToken, configPtrToken, configValueToken, factoryToken},
	})
	return &Module{BaseModule: base, bus: bus, config: cfg}, nil
}

// ForRootCompat 显式保留 standalone Init 写入全局 registry 的兼容语义。
func ForRootCompat(cfg Config) (module.Module, error) {
	bus, err := New(cfg)
	if err != nil {
		return nil, err
	}
	busToken := reflect.TypeFor[*Bus]()
	emitterToken := reflect.TypeFor[Emitter]()
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()
	lifecycleToken := reflect.TypeFor[interface{ OnApplicationBootstrap() error }]()

	pBus, err := di.ProviderFromInstanceWithToken(busToken, bus, di.Singleton)
	if err != nil {
		return nil, err
	}
	pEmitter, err := di.ProviderFromInstanceWithToken(emitterToken, Emitter(bus), di.Singleton)
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
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(bus), di.Singleton)
	if err != nil {
		return nil, err
	}
	pLifecycle, err := di.ProviderFromInstanceWithToken(lifecycleToken, NewLifecycleHook(bus, nil), di.Singleton)
	if err != nil {
		return nil, err
	}
	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pBus, pEmitter, pConfigPtr, pConfigValue, pFactory, pLifecycle},
		Exports:   []interface{}{busToken, emitterToken, configPtrToken, configValueToken, factoryToken},
	})
	return &Module{compatSource: "integrations/events.ForRootCompat", BaseModule: base, bus: bus, config: cfg}, nil
}

// Init 初始化事件模块，并把相关 container binding 注册到 registry。
func (m *Module) Init() error {
	if err := m.BaseModule.Init(); err != nil {
		return err
	}
	reg := m.reg
	if reg == nil {
		if m.compatSource == "" {
			return fmt.Errorf("events.Module.Init requires an explicit registry; call SetRegistry(...) before Init or use events.ForRootCompat(...) for standalone/global compatibility")
		}
		reg = registry.GlobalWithUsage(m.compatSource)
	}
	coreintegration.RegisterContainerBindings(reg,
		coreintegration.NewBindingEntryFor[*Bus]("EventsBus"),
		coreintegration.NewBindingEntry("EventsEmitter", reflect.TypeFor[Emitter]()),
		coreintegration.NewBindingEntry("EventsConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("EventsConfigPtr", reflect.TypeFor[*Config]()),
		coreintegration.NewBindingEntry("EventsFactory", reflect.TypeFor[Factory]()),
	)
	return nil
}

// Bus 返回当前模块持有的事件总线。
func (m *Module) Bus() *Bus { return m.bus }

// Config 返回当前模块的配置快照。
func (m *Module) Config() Config { return m.config }

// SetRegistry 注入 registry，供 Init 阶段注册绑定声明。
func (m *Module) SetRegistry(reg *registry.Registry) { m.reg = reg }
