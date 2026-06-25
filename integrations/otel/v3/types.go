// types.go 定义 otel 集成对外暴露的公共类型、选项与包装结构。
package otel

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	// DefaultName 是默认 Telemetry 实例名。
	DefaultName                = "default"
	// DefaultServiceName 是默认服务名。
	DefaultServiceName         = "lania-g"
	// DefaultInstrumentationName 是默认 instrumentation 名称。
	DefaultInstrumentationName = "github.com/sao-lang/lania-g/integrations/otel/v3"
	// DefaultRequestIDHeader 是默认请求 ID 请求头名。
	DefaultRequestIDHeader     = "X-Request-Id"
	// DefaultTraceparentHeader 是默认 traceparent 请求头名。
	DefaultTraceparentHeader   = "traceparent"
	// DefaultRequestCounterName 是默认请求计数指标名。
	DefaultRequestCounterName  = "lania.request.count"
	// DefaultRequestDurationName 是默认请求耗时指标名。
	DefaultRequestDurationName = "lania.request.duration"
	// DefaultErrorCounterName 是默认错误计数指标名。
	DefaultErrorCounterName    = "lania.error.count"
	// DefaultThroughputName 是默认吞吐指标名。
	DefaultThroughputName      = "lania.throughput"

	// MetadataKeyTraceContext 是在 runtime metadata 中写入 TraceContext 的键。
	MetadataKeyTraceContext = "otel.trace_context"
	// MetadataKeyTraceID 是在 runtime metadata 中写入 trace id 的键。
	MetadataKeyTraceID      = "otel.trace_id"
	// MetadataKeySpanID 是在 runtime metadata 中写入 span id 的键。
	MetadataKeySpanID       = "otel.span_id"
	// MetadataKeyRequestID 是在 runtime metadata 中写入 request id 的键。
	MetadataKeyRequestID    = "otel.request_id"
)

// ExporterConfig 描述一个 exporter 的初始化参数。
type ExporterConfig struct {
	Type      string
	Endpoint  string
	Sampler   float64
	BatchSize int
	Interval  time.Duration
	Headers   map[string]string
}

// MetricConfig 定义内置指标名称与公共标签。
type MetricConfig struct {
	RequestDurationName string
	ErrorCounterName    string
	ThroughputName      string
	Labels              map[string]string
}

// Config 描述 OpenTelemetry integration 的初始化配置。
type Config struct {
	Name                string
	ServiceName         string
	InstrumentationName string
	RequestIDHeader     string
	TraceparentHeader   string
	RequestCounterName  string
	Propagator          propagation.TextMapPropagator
	TracerProvider      oteltrace.TracerProvider
	MeterProvider       metric.MeterProvider
	Exporters           []ExporterConfig
	Metrics             MetricConfig
	Resource            *resource.Resource
	BatchProcessor      sdktrace.SpanProcessor
}

// Factory 定义 Telemetry 工厂接口，便于通过 DI 获取默认实例或按配置创建新实例。
type Factory interface {
	Default() *Telemetry
	New(cfg Config) (*Telemetry, error)
}

// Telemetry 封装 tracing、metrics 与传播器等运行期对象。
type Telemetry struct {
	config            Config
	propagator        propagation.TextMapPropagator
	tracerProvider    oteltrace.TracerProvider
	meterProvider     metric.MeterProvider
	tracer            oteltrace.Tracer
	meter             metric.Meter
	requestCounter    metric.Int64Counter
	requestDuration   metric.Float64Histogram
	errorCounter      metric.Int64Counter
	throughputCounter metric.Int64Counter
	commonAttrs       []attribute.KeyValue
	exporters         []sdktrace.SpanExporter
	spanProcessors    []sdktrace.SpanProcessor
	resource          *resource.Resource
}

// TraceContext 表示当前请求或调用链上的追踪上下文快照。
type TraceContext struct {
	TraceID   string
	SpanID    string
	RequestID string
	Remote    bool
	Sampled   bool
}

type contextKey string

const (
	// ContextKeyTraceID 是在 context 中保存 trace id 的键。
	ContextKeyTraceID   contextKey = "otel.trace_id"
	// ContextKeySpanID 是在 context 中保存 span id 的键。
	ContextKeySpanID    contextKey = "otel.span_id"
	// ContextKeyRequestID 是在 context 中保存 request id 的键。
	ContextKeyRequestID contextKey = "otel.request_id"
)

// LoggerContextKeys 返回建议写入日志上下文的 trace/request 相关键名。
func LoggerContextKeys() []string {
	return []string{
		string(ContextKeyTraceID),
		string(ContextKeySpanID),
		string(ContextKeyRequestID),
	}
}

// DefaultConfig 返回一份适合直接启用的默认配置。
func DefaultConfig() Config {
	return Config{
		Name:                DefaultName,
		ServiceName:         DefaultServiceName,
		InstrumentationName: DefaultInstrumentationName,
		RequestIDHeader:     DefaultRequestIDHeader,
		TraceparentHeader:   DefaultTraceparentHeader,
		RequestCounterName:  DefaultRequestCounterName,
		Metrics: MetricConfig{
			RequestDurationName: DefaultRequestDurationName,
			ErrorCounterName:    DefaultErrorCounterName,
			ThroughputName:      DefaultThroughputName,
			Labels:              make(map[string]string),
		},
		Exporters: []ExporterConfig{},
	}
}

func normalizeConfig(cfg Config) Config {
	def := DefaultConfig()
	if cfg.Name == "" {
		cfg.Name = def.Name
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = def.ServiceName
	}
	if cfg.InstrumentationName == "" {
		cfg.InstrumentationName = def.InstrumentationName
	}
	if cfg.RequestIDHeader == "" {
		cfg.RequestIDHeader = def.RequestIDHeader
	}
	if cfg.TraceparentHeader == "" {
		cfg.TraceparentHeader = def.TraceparentHeader
	}
	if cfg.RequestCounterName == "" {
		cfg.RequestCounterName = def.RequestCounterName
	}
	if cfg.Metrics.RequestDurationName == "" {
		cfg.Metrics.RequestDurationName = def.Metrics.RequestDurationName
	}
	if cfg.Metrics.ErrorCounterName == "" {
		cfg.Metrics.ErrorCounterName = def.Metrics.ErrorCounterName
	}
	if cfg.Metrics.ThroughputName == "" {
		cfg.Metrics.ThroughputName = def.Metrics.ThroughputName
	}
	if cfg.Metrics.Labels == nil {
		cfg.Metrics.Labels = make(map[string]string)
	}
	if cfg.Propagator == nil {
		cfg.Propagator = propagation.TraceContext{}
	}
	if cfg.Resource == nil {
		cfg.Resource = resource.NewWithAttributes(
			"https://opentelemetry.io/schemas/1.17.0",
			attribute.String("service.name", cfg.ServiceName),
			attribute.String("service.version", "1.0.0"),
		)
	}
	if cfg.TracerProvider == nil {
		cfg.TracerProvider = newTracerProvider(cfg)
	}
	if cfg.MeterProvider == nil {
		cfg.MeterProvider = newMeterProvider(cfg)
	}
	return cfg
}

// New 基于配置创建一个 Telemetry 实例，并初始化默认 tracer/meter/指标句柄。
func New(cfg Config) (*Telemetry, error) {
	cfg = normalizeConfig(cfg)

	tracer := cfg.TracerProvider.Tracer(cfg.InstrumentationName)
	meter := cfg.MeterProvider.Meter(cfg.InstrumentationName)

	requestCounter, err := meter.Int64Counter(cfg.RequestCounterName)
	if err != nil {
		return nil, err
	}

	requestDuration, err := meter.Float64Histogram(cfg.Metrics.RequestDurationName)
	if err != nil {
		return nil, err
	}

	errorCounter, err := meter.Int64Counter(cfg.Metrics.ErrorCounterName)
	if err != nil {
		return nil, err
	}

	throughputCounter, err := meter.Int64Counter(cfg.Metrics.ThroughputName)
	if err != nil {
		return nil, err
	}

	commonAttrs := []attribute.KeyValue{
		attribute.String("service.name", cfg.ServiceName),
		attribute.String("lania.integration", "otel"),
	}
	for k, v := range cfg.Metrics.Labels {
		commonAttrs = append(commonAttrs, attribute.String(k, v))
	}

	spanProcessors := make([]sdktrace.SpanProcessor, 0, 1)
	if cfg.BatchProcessor != nil {
		spanProcessors = append(spanProcessors, cfg.BatchProcessor)
	}

	return &Telemetry{
		config:            cfg,
		propagator:        cfg.Propagator,
		tracerProvider:    cfg.TracerProvider,
		meterProvider:     cfg.MeterProvider,
		tracer:            tracer,
		meter:             meter,
		requestCounter:    requestCounter,
		requestDuration:   requestDuration,
		errorCounter:      errorCounter,
		throughputCounter: throughputCounter,
		commonAttrs:       commonAttrs,
		exporters:         nil,
		spanProcessors:    spanProcessors,
		resource:          cfg.Resource,
	}, nil
}

func newTracerProvider(cfg Config) oteltrace.TracerProvider {
	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(cfg.Resource),
	}
	if cfg.BatchProcessor != nil {
		options = append(options, sdktrace.WithSpanProcessor(cfg.BatchProcessor))
	}
	return sdktrace.NewTracerProvider(options...)
}

func newMeterProvider(cfg Config) metric.MeterProvider {
	return sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(cfg.Resource),
	)
}

// Default 返回当前 Telemetry 本身，用于满足 Factory 接口。
func (t *Telemetry) Default() *Telemetry { return t }

// New 基于 cfg 创建一个新的 Telemetry 实例，用于满足 Factory 接口。
func (t *Telemetry) New(cfg Config) (*Telemetry, error) { return New(cfg) }

// Config 返回归一化后的配置。
func (t *Telemetry) Config() Config { return t.config }

// Tracer 返回默认 tracer。
func (t *Telemetry) Tracer() oteltrace.Tracer { return t.tracer }

// Meter 返回默认 meter。
func (t *Telemetry) Meter() metric.Meter { return t.meter }

// Propagator 返回文本传播器。
func (t *Telemetry) Propagator() propagation.TextMapPropagator { return t.propagator }

// TracerProvider 返回 tracer provider。
func (t *Telemetry) TracerProvider() oteltrace.TracerProvider { return t.tracerProvider }

// MeterProvider 返回 meter provider。
func (t *Telemetry) MeterProvider() metric.MeterProvider { return t.meterProvider }

// RequestCounter 返回请求计数器。
func (t *Telemetry) RequestCounter() metric.Int64Counter { return t.requestCounter }

// RequestDuration 返回请求耗时直方图。
func (t *Telemetry) RequestDuration() metric.Float64Histogram { return t.requestDuration }

// ErrorCounter 返回错误计数器。
func (t *Telemetry) ErrorCounter() metric.Int64Counter { return t.errorCounter }

// ThroughputCounter 返回吞吐量计数器。
func (t *Telemetry) ThroughputCounter() metric.Int64Counter { return t.throughputCounter }

// Exporters 返回底层 span exporter 列表。
func (t *Telemetry) Exporters() []sdktrace.SpanExporter { return t.exporters }

// SpanProcessors 返回底层 span processor 列表。
func (t *Telemetry) SpanProcessors() []sdktrace.SpanProcessor { return t.spanProcessors }

// Resource 返回 OTel resource。
func (t *Telemetry) Resource() *resource.Resource { return t.resource }

// ContextWithTraceContext 把 TraceContext 中的关键字段写入 context。
func ContextWithTraceContext(ctx context.Context, tc TraceContext) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if tc.TraceID != "" {
		ctx = context.WithValue(ctx, ContextKeyTraceID, tc.TraceID)
	}
	if tc.SpanID != "" {
		ctx = context.WithValue(ctx, ContextKeySpanID, tc.SpanID)
	}
	if tc.RequestID != "" {
		ctx = context.WithValue(ctx, ContextKeyRequestID, tc.RequestID)
	}
	return ctx
}

// TraceContextFromContext 从 context 中提取 TraceContext。
func TraceContextFromContext(ctx context.Context) TraceContext {
	if ctx == nil {
		return TraceContext{}
	}
	tc := TraceContext{}
	if value, ok := ctx.Value(ContextKeyTraceID).(string); ok {
		tc.TraceID = value
	}
	if value, ok := ctx.Value(ContextKeySpanID).(string); ok {
		tc.SpanID = value
	}
	if value, ok := ctx.Value(ContextKeyRequestID).(string); ok {
		tc.RequestID = value
	}
	return tc
}
