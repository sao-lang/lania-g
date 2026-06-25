// runtime_interceptor.go 实现 otel 集成的拦截器侧运行时接入逻辑。
package otel

import (
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// Interceptor 返回一个用于追踪与指标采集的 AOP interceptor。
func (t *Telemetry) Interceptor() aop.InterceptorFunc {
	return func(execCtx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
		hc, ok := execCtx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return next.Handle()
		}
		startTime := time.Now()
		spanName := spanName(execCtx, hc)
		attrs := append([]attribute.KeyValue{}, t.commonAttrs...)
		attrs = append(attrs,
			attribute.String("lania.protocol", string(hc.Protocol)),
			attribute.String("lania.route_key", hc.RouteKey),
			attribute.String("http.method", hc.Request.Method),
			attribute.String("url.path", hc.Request.Path),
		)
		ctx, span := t.tracer.Start(hc.Context(), spanName, oteltrace.WithAttributes(attrs...))
		defer span.End()
		hc.WithContext(ctx)
		snapshot := snapshotFromSpanContext(span.SpanContext(), Current(hc).RequestID)
		applyTraceContext(hc, snapshot)
		result, err := next.Handle()
		duration := time.Since(startTime).Seconds()
		t.requestCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
		t.throughputCounter.Add(ctx, 1, metric.WithAttributes(attrs...))
		t.requestDuration.Record(ctx, duration, metric.WithAttributes(attrs...))
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			t.errorCounter.Add(ctx, 1, metric.WithAttributes(append(attrs, attribute.String("error", err.Error()))...))
			return nil, err
		}
		statusAttrs := append(attrs, attribute.Int("http.status_code", hc.Response.Status))
		span.SetAttributes(statusAttrs...)
		return result, nil
	}
}
