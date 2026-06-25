// runtime_middleware.go 实现 otel 集成的中间件侧运行时接入逻辑。
package otel

import (
	"context"
	"strings"

	"github.com/google/uuid"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// Middleware 返回一个用于提取 trace 上下文、生成 request id，并写入 runtime context 的 AOP middleware。
func (t *Telemetry) Middleware() aop.MiddlewareFunc {
	return func(execCtx *aop.ExecutionContext, next func() error) error {
		hc, ok := execCtx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return next()
		}
		baseCtx := hc.Context()
		if baseCtx == nil {
			baseCtx = context.Background()
		}
		extractedCtx := t.propagator.Extract(baseCtx, headerCarrier{headers: hc.Request.Headers})
		requestID := firstHeaderValue(hc, t.config.RequestIDHeader)
		if requestID == "" {
			requestID = uuid.NewString()
		}
		snapshot := snapshotFromSpanContext(oteltrace.SpanContextFromContext(extractedCtx), requestID)
		hc.WithContext(ContextWithTraceContext(extractedCtx, snapshot))
		applyTraceContext(hc, snapshot)
		if traceparent := firstHeaderValue(hc, t.config.TraceparentHeader); traceparent != "" {
			hc.Response.Headers[t.config.TraceparentHeader] = traceparent
		}
		if header := strings.TrimSpace(t.config.RequestIDHeader); header != "" {
			hc.Response.Headers[header] = requestID
		}
		return next()
	}
}
