// module.go 负责把 events integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package events

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

// Module 是事件集成对应的模块封装。
type integrationModule struct {
	*module.BaseModule
	bus          *Bus
	config       Config
}

// ForRoot 创建事件集成模块，并注册事件总线、发射器、配置和工厂。
func build(cfg Config) (module.Module, error) {
	bus, err := New(cfg)
	if err != nil {
		return nil, err
	}
	busToken := reflect.TypeFor[*Bus]()
	emitterToken := reflect.TypeFor[Emitter]()
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()

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
	base := module.New(module.Metadata{
		Providers: []*di.Provider{pBus, pEmitter, pConfigPtr, pConfigValue, pFactory},
		Exports:   []interface{}{busToken, emitterToken, configPtrToken, configValueToken, factoryToken},
	})
	return &integrationModule{BaseModule: base, bus: bus, config: cfg}, nil
}

// Init 初始化事件模块，并把相关 container binding 注册到 registry。
// Module 创建 events 集成模块。
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
		coreintegration.NewBindingEntryFor[*Bus]("EventsBus"),
		coreintegration.NewBindingEntry("EventsEmitter", reflect.TypeFor[Emitter]()),
		coreintegration.NewBindingEntry("EventsConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("EventsConfigPtr", reflect.TypeFor[*Config]()),
		coreintegration.NewBindingEntry("EventsFactory", reflect.TypeFor[Factory]()),
	)
	return nil
}

// Bus 返回当前模块持有的事件总线。
func (m *integrationModule) Bus() *Bus { return m.bus }

// Config 返回当前模块的配置快照。
func (m *integrationModule) Config() Config { return m.config }

// Declare 收集实现了 Subscriber 的控制器，并把订阅绑定到事件总线。
//
// 与 HTTP 一样，订阅声明归属于控制器所在的模块。
func (m *integrationModule) Declare(root *module.ModuleRef) error {
	if root == nil || m.bus == nil {
		return nil
	}
	var errs []error
	err := module.WalkModules(root.GetRootModule(), func(mod module.Module) error {
		metadata := mod.Metadata()
		if metadata == nil {
			return nil
		}
		for _, controller := range metadata.Controllers {
			instance, err := module.ResolveController(mod, controller)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			subscriber, ok := instance.(Subscriber)
			if !ok {
				continue
			}
			routes := &Routes{receiver: instance}
			subscriber.Subscribe(routes)
			if err := routes.bind(m.bus); err != nil {
				errs = append(errs, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("events subscription failed: %v", errs)
	}
	return nil
}
