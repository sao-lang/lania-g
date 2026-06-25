// runtime_carrier.go 实现 otel 集成在 runtime 内部传播上下文所需的 carrier 封装。
package otel

import (
	"strings"

	"go.opentelemetry.io/otel/propagation"
)

type headerCarrier struct {
	headers map[string]string
}

// Get 按 key 获取 header 值（大小写不敏感）。
func (c headerCarrier) Get(key string) string {
	for name, value := range c.headers {
		if strings.EqualFold(name, key) {
			return value
		}
	}
	return ""
}

// Set 设置 header 值。
func (c headerCarrier) Set(key, value string) {
	if c.headers == nil {
		return
	}
	c.headers[key] = value
}

// Keys 返回全部 header key。
func (c headerCarrier) Keys() []string {
	keys := make([]string, 0, len(c.headers))
	for key := range c.headers {
		keys = append(keys, key)
	}
	return keys
}

var _ propagation.TextMapCarrier = headerCarrier{}
