// util.go 提供 resilience 集成内部复用的辅助函数。
package resilience

import (
	"maps"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

func routeKeyOf(hc *runtime.HandlerContext) string {
	if hc != nil && hc.RouteKey != "" {
		return hc.RouteKey
	}
	return "unknown"
}

func firstHeader(hc *runtime.HandlerContext, name string) string {
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
	if value, ok := hc.Get(name); ok {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

func cloneHeaders(src map[string]string) map[string]string {
	out := make(map[string]string, len(src))
	maps.Copy(out, src)
	return out
}
