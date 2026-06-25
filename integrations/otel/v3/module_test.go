package otel

import (
	"context"
	"reflect"
	"strings"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/application/v3/factory"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	"github.com/sao-lang/lania-g/integrations/logger/v3"
)

type auditTelemetry struct{}

func (auditTelemetry) TelemetryName() string { return "audit" }

type otelTestHandler struct{}

func (h *otelTestHandler) Handle() string { return "pong" }

func TestForRoot_RegistersTelemetryFactoryAndBindings(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	tracerProvider := sdktrace.NewTracerProvider()

	mod, err := ForRoot(Config{
		Name:           "audit",
		ServiceName:    "svc-audit",
		MeterProvider:  meterProvider,
		TracerProvider: tracerProvider,
	})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(interface{ SetRegistry(*registry.Registry) }).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	telemetryAny, err := mod.Container().Get(reflect.TypeFor[*Telemetry]())
	if err != nil {
		t.Fatalf("get telemetry: %v", err)
	}
	telemetry := telemetryAny.(*Telemetry)
	if telemetry.Config().ServiceName != "svc-audit" {
		t.Fatalf("service name = %q", telemetry.Config().ServiceName)
	}

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}
	ctx := runtime.NewHandlerContext("test")
	ctx.Container = mod.Container().NewChild()

	refType := reflect.TypeOf(TelemetryRef[auditTelemetry]{})
	handler := &runtime.Handler{
		Meta: &runtime.HandlerMeta{ParamPlans: []runtime.ParamPlan{{Index: 0, Type: refType}}},
	}
	value, err := br.Resolve(ctx, handler, refType, 0)
	if err != nil {
		t.Fatalf("resolve telemetry ref: %v", err)
	}
	ref := value.(TelemetryRef[auditTelemetry])
	if ref.Telemetry == nil || ref.Config().Name != "audit" {
		t.Fatalf("resolved telemetry ref invalid")
	}
}

func TestMiddlewareAndInterceptor_PropagateTraceAndMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	tracerProvider := sdktrace.NewTracerProvider()

	telemetry, err := New(Config{
		ServiceName:    "svc-otel",
		MeterProvider:  meterProvider,
		TracerProvider: tracerProvider,
	})
	if err != nil {
		t.Fatalf("new telemetry: %v", err)
	}

	handler, err := runtime.NewHandler(&otelTestHandler{}, "Handle")
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	handler.Meta.RouteKey = runtime.BuildRouteKey("http", "GET", "/ping")
	handler.Meta.Protocol = "http"

	rt := runtime.NewRuntime()
	rt.GetRouter().Register(handler.Meta.RouteKey, handler)
	rt.UseGlobalMiddleware(Middleware(telemetry))
	rt.UseGlobalInterceptors(Interceptor(telemetry))

	ctx := runtime.NewHandlerContext("http")
	ctx.Request.Method = "GET"
	ctx.Request.Path = "/ping"
	ctx.Request.Headers["traceparent"] = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	ctx.Request.Headers["X-Request-Id"] = "req-123"

	out, err := rt.Execute(ctx)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.(string) != "pong" {
		t.Fatalf("out = %v", out)
	}

	tc := Current(ctx)
	if tc.RequestID != "req-123" {
		t.Fatalf("request id = %q", tc.RequestID)
	}
	if tc.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace id = %q", tc.TraceID)
	}
	if tc.SpanID == "" {
		t.Fatalf("span id should not be empty")
	}
	if ctx.Response.Headers["X-Request-Id"] != "req-123" {
		t.Fatalf("response request id = %q", ctx.Response.Headers["X-Request-Id"])
	}
	if ctx.Response.Headers["traceparent"] != "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01" {
		t.Fatalf("response traceparent = %q", ctx.Response.Headers["traceparent"])
	}

	fields := LoggerFields(ctx)
	if len(fields) != 3 {
		t.Fatalf("logger fields len = %d", len(fields))
	}
	derived := WithLoggerFields(testLogger{}, ctx)
	if derived == nil {
		t.Fatalf("derived logger nil")
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	if got := requestCount(rm, telemetry.Config().RequestCounterName); got != 1 {
		t.Fatalf("request count = %d, want 1", got)
	}
}

func TestBindings_ResolveTraceContextAndRequestID(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRoot(Config{})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(interface{ SetRegistry(*registry.Registry) }).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}

	ctx := runtime.NewHandlerContext("http")
	ctx.Container = mod.Container().NewChild()
	ctx.Set(MetadataKeyTraceContext, TraceContext{
		TraceID:   "trace-1",
		SpanID:    "span-1",
		RequestID: "req-1",
	})

	traceType := reflect.TypeOf(InjectTraceContext{})
	traceValue, err := br.Resolve(ctx, nil, traceType, 0)
	if err != nil {
		t.Fatalf("resolve trace context: %v", err)
	}
	traceCtx := traceValue.(InjectTraceContext)
	if traceCtx.TraceID != "trace-1" || traceCtx.RequestID != "req-1" {
		t.Fatalf("trace context = %+v", traceCtx.TraceContext)
	}

	requestType := reflect.TypeOf(InjectRequestID{})
	requestValue, err := br.Resolve(ctx, nil, requestType, 1)
	if err != nil {
		t.Fatalf("resolve request id: %v", err)
	}
	requestID := requestValue.(InjectRequestID)
	if requestID.Value != "req-1" {
		t.Fatalf("request id = %q", requestID.Value)
	}
}

func TestInstallHelpers(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	tracerProvider := sdktrace.NewTracerProvider()
	telemetry, err := New(Config{
		ServiceName:    "svc-otel",
		MeterProvider:  meterProvider,
		TracerProvider: tracerProvider,
	})
	if err != nil {
		t.Fatalf("new telemetry: %v", err)
	}

	app := &installTarget{}
	Install(app, telemetry)
	if app.middlewareCount != 1 || app.interceptorCount != 1 {
		t.Fatalf("application install counts = %d/%d", app.middlewareCount, app.interceptorCount)
	}

	f := factory.New()
	if err := InstallOnFactory(f, telemetry); err != nil {
		t.Fatalf("install on factory: %v", err)
	}
}

type testLogger struct{}

type installTarget struct {
	middlewareCount  int
	interceptorCount int
}

func (t *installTarget) UseGlobalMiddlewares(items ...aop.MiddlewareFunc) {
	t.middlewareCount += len(items)
}

func (t *installTarget) UseGlobalInterceptors(items ...aop.InterceptorFunc) {
	t.interceptorCount += len(items)
}

func (testLogger) Debug(msg string, fields ...logger.Field)      {}
func (testLogger) Info(msg string, fields ...logger.Field)       {}
func (testLogger) Warn(msg string, fields ...logger.Field)       {}
func (testLogger) Error(msg string, fields ...logger.Field)      {}
func (testLogger) Fatal(msg string, fields ...logger.Field)      {}
func (testLogger) Debugf(format string, args ...interface{})     {}
func (testLogger) Infof(format string, args ...interface{})      {}
func (testLogger) Warnf(format string, args ...interface{})      {}
func (testLogger) Errorf(format string, args ...interface{})     {}
func (testLogger) Fatalf(format string, args ...interface{})     {}
func (testLogger) With(fields ...logger.Field) logger.Logger     { return testLogger{} }
func (testLogger) WithContext(ctx context.Context) logger.Logger { return testLogger{} }
func (testLogger) Sync() error                                   { return nil }

func requestCount(rm metricdata.ResourceMetrics, metricName string) int64 {
	for _, scope := range rm.ScopeMetrics {
		for _, item := range scope.Metrics {
			if item.Name != metricName {
				continue
			}
			sum, ok := item.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			var total int64
			for _, point := range sum.DataPoints {
				total += point.Value
			}
			return total
		}
	}
	return 0
}

func TestForRoot_InitRequiresExplicitRegistry(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRoot(Config{})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	if err := mod.Init(); err == nil {
		t.Fatalf("expected missing registry error")
	} else if !strings.Contains(err.Error(), "requires an explicit registry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestForRootCompat_InitRoutesToCompatFallbackSource(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRootCompat(Config{})
	if err != nil {
		t.Fatalf("for root compat: %v", err)
	}
	if err := mod.Init(); err != nil {
		t.Fatalf("init compat module: %v", err)
	}
	if got := registry.Global().SnapshotFallbackUsage()["integrations/otel.ForRootCompat"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}
