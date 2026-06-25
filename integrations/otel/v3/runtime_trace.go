// runtime_trace.go 实现 otel 集成的 trace 创建、传播与收口辅助。
package otel

import (
	"strings"

	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	"github.com/sao-lang/lania-g/integrations/logger/v3"
)

// Current 返回当前请求的 TraceContext 快照。
func Current(hc *runtime.HandlerContext) TraceContext {
	if hc == nil {
		return TraceContext{}
	}
	if value, ok := hc.Get(MetadataKeyTraceContext); ok {
		if tc, ok := value.(TraceContext); ok {
			return tc
		}
	}
	return TraceContextFromContext(hc.Context())
}

// LoggerFields 返回适合追加到日志的 trace/request 相关字段。
func LoggerFields(hc *runtime.HandlerContext) []logger.Field {
	tc := Current(hc)
	fields := make([]logger.Field, 0, 3)
	if tc.RequestID != "" {
		fields = append(fields, logger.String("request_id", tc.RequestID))
	}
	if tc.TraceID != "" {
		fields = append(fields, logger.String("trace_id", tc.TraceID))
	}
	if tc.SpanID != "" {
		fields = append(fields, logger.String("span_id", tc.SpanID))
	}
	return fields
}

// WithLoggerFields 将 trace/request 相关字段追加到 logger 上。
func WithLoggerFields(lg logger.Logger, hc *runtime.HandlerContext) logger.Logger {
	if lg == nil {
		return nil
	}
	fields := LoggerFields(hc)
	if len(fields) == 0 {
		return lg
	}
	return lg.With(fields...)
}

func applyTraceContext(hc *runtime.HandlerContext, tc TraceContext) {
	hc.Set(MetadataKeyTraceContext, tc)
	hc.Set(MetadataKeyTraceID, tc.TraceID)
	hc.Set(MetadataKeySpanID, tc.SpanID)
	hc.Set(MetadataKeyRequestID, tc.RequestID)
	hc.WithContext(ContextWithTraceContext(hc.Context(), tc))
}

func snapshotFromSpanContext(sc oteltrace.SpanContext, requestID string) TraceContext {
	tc := TraceContext{RequestID: requestID}
	if sc.IsValid() {
		tc.TraceID = sc.TraceID().String()
		tc.SpanID = sc.SpanID().String()
		tc.Remote = sc.IsRemote()
		tc.Sampled = sc.IsSampled()
	}
	return tc
}

func spanName(execCtx *aop.ExecutionContext, hc *runtime.HandlerContext) string {
	if execCtx != nil && execCtx.RouteKey != "" {
		return execCtx.RouteKey
	}
	if hc != nil && hc.RouteKey != "" {
		return hc.RouteKey
	}
	if hc != nil && hc.Request.Path != "" {
		return string(hc.Protocol) + " " + hc.Request.Path
	}
	return "lania.request"
}

func firstHeaderValue(hc *runtime.HandlerContext, name string) string {
	if hc == nil || name == "" {
		return ""
	}
	for key, value := range hc.Request.Headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	for key, values := range hc.Request.HeadersMulti {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}
