// module.go 负责把 otel integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package otel

import (
	"fmt"
	"reflect"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// Module 是 OpenTelemetry integration 的模块封装。
//
// 它会根据配置创建 `Telemetry`，并把 Telemetry/Factory/Config 等作为 provider 导出，
// 同时注册注入 wrapper（见 refs.go）。
type Module struct {
	*module.BaseModule
	telemetry    *Telemetry
	config       Config
	reg          *registry.Registry
	compatSource string
}

// ForRoot 创建一个启用 OpenTelemetry 的模块。
//
// 返回的 module 会导出：
// - `*Telemetry`（默认与按 name 的 token 版本）
// - `Config` / `*Config`
// - `Factory`
// - TracerProvider / MeterProvider / Propagator
func ForRoot(cfg Config) (module.Module, error) {
	telemetry, err := New(cfg)
	if err != nil {
		return nil, err
	}
	cfg = telemetry.Config()

	telemetryToken := reflect.TypeFor[*Telemetry]()
	namedTelemetryToken := TelemetryToken(cfg.Name)
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()
	tracerProviderToken := reflect.TypeFor[oteltrace.TracerProvider]()
	meterProviderToken := reflect.TypeFor[metric.MeterProvider]()
	propagatorToken := reflect.TypeFor[propagation.TextMapPropagator]()

	pTelemetry, err := di.ProviderFromInstanceWithToken(telemetryToken, telemetry, di.Singleton)
	if err != nil {
		return nil, err
	}
	pNamedTelemetry, err := di.ProviderFromInstanceWithToken(namedTelemetryToken, telemetry, di.Singleton)
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
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(telemetry), di.Singleton)
	if err != nil {
		return nil, err
	}
	pTracerProvider, err := di.ProviderFromInstanceWithToken(tracerProviderToken, cfg.TracerProvider, di.Singleton)
	if err != nil {
		return nil, err
	}
	pMeterProvider, err := di.ProviderFromInstanceWithToken(meterProviderToken, cfg.MeterProvider, di.Singleton)
	if err != nil {
		return nil, err
	}
	pPropagator, err := di.ProviderFromInstanceWithToken(propagatorToken, cfg.Propagator, di.Singleton)
	if err != nil {
		return nil, err
	}

	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{
			pTelemetry,
			pNamedTelemetry,
			pConfigPtr,
			pConfigValue,
			pFactory,
			pTracerProvider,
			pMeterProvider,
			pPropagator,
		},
		Exports: []interface{}{
			telemetryToken,
			namedTelemetryToken,
			configPtrToken,
			configValueToken,
			factoryToken,
			tracerProviderToken,
			meterProviderToken,
			propagatorToken,
		},
	})
	return &Module{BaseModule: base, telemetry: telemetry, config: cfg}, nil
}

// ForRootCompat 显式保留 standalone Init 写入全局 registry 的兼容语义。
func ForRootCompat(cfg Config) (module.Module, error) {
	telemetry, err := New(cfg)
	if err != nil {
		return nil, err
	}
	cfg = telemetry.Config()

	telemetryToken := reflect.TypeFor[*Telemetry]()
	namedTelemetryToken := TelemetryToken(cfg.Name)
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()
	tracerProviderToken := reflect.TypeFor[oteltrace.TracerProvider]()
	meterProviderToken := reflect.TypeFor[metric.MeterProvider]()
	propagatorToken := reflect.TypeFor[propagation.TextMapPropagator]()

	pTelemetry, err := di.ProviderFromInstanceWithToken(telemetryToken, telemetry, di.Singleton)
	if err != nil {
		return nil, err
	}
	pNamedTelemetry, err := di.ProviderFromInstanceWithToken(namedTelemetryToken, telemetry, di.Singleton)
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
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(telemetry), di.Singleton)
	if err != nil {
		return nil, err
	}
	pTracerProvider, err := di.ProviderFromInstanceWithToken(tracerProviderToken, cfg.TracerProvider, di.Singleton)
	if err != nil {
		return nil, err
	}
	pMeterProvider, err := di.ProviderFromInstanceWithToken(meterProviderToken, cfg.MeterProvider, di.Singleton)
	if err != nil {
		return nil, err
	}
	pPropagator, err := di.ProviderFromInstanceWithToken(propagatorToken, cfg.Propagator, di.Singleton)
	if err != nil {
		return nil, err
	}

	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{
			pTelemetry,
			pNamedTelemetry,
			pConfigPtr,
			pConfigValue,
			pFactory,
			pTracerProvider,
			pMeterProvider,
			pPropagator,
		},
		Exports: []interface{}{
			telemetryToken,
			namedTelemetryToken,
			configPtrToken,
			configValueToken,
			factoryToken,
			tracerProviderToken,
			meterProviderToken,
			propagatorToken,
		},
	})
	return &Module{compatSource: "integrations/otel.ForRootCompat", BaseModule: base, telemetry: telemetry, config: cfg}, nil
}

// Init 注册该模块的 binding 与注入包装，并完成 BaseModule 初始化。
func (m *Module) Init() error {
	if err := m.BaseModule.Init(); err != nil {
		return err
	}
	reg := m.reg
	if reg == nil {
		if m.compatSource == "" {
			return fmt.Errorf("otel.Module.Init requires an explicit registry; call SetRegistry(...) before Init or use otel.ForRootCompat(...) for standalone/global compatibility")
		}
		reg = registry.GlobalWithUsage(m.compatSource)
	}
	coreintegration.RegisterContainerBindings(reg,
		coreintegration.NewBindingEntryFor[*Telemetry]("OTel"),
		coreintegration.NewBindingEntry("OTelFactory", reflect.TypeFor[Factory]()),
		coreintegration.NewBindingEntry("OTelConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("OTelConfigPtr", reflect.TypeFor[*Config]()),
		coreintegration.NewBindingEntry("OTelTracerProvider", reflect.TypeFor[oteltrace.TracerProvider]()),
		coreintegration.NewBindingEntry("OTelMeterProvider", reflect.TypeFor[metric.MeterProvider]()),
		coreintegration.NewBindingEntry("OTelPropagator", reflect.TypeFor[propagation.TextMapPropagator]()),
	)
	RegisterBindings(reg)
	return nil
}

// Telemetry 返回模块创建的 Telemetry 实例。
func (m *Module) Telemetry() *Telemetry { return m.telemetry }

// Config 返回模块的最终配置（已归一化）。
func (m *Module) Config() Config { return m.config }

// SetRegistry 为 Init 阶段指定要写入的 registry；为空时使用全局 registry。
func (m *Module) SetRegistry(reg *registry.Registry) { m.reg = reg }
