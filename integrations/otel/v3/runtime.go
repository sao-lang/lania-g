// runtime.go 实现 otel 集成与框架 runtime 之间的接入逻辑。
package otel

import "github.com/sao-lang/lania-g/kernel/v3/aop"

// Middleware 返回 Telemetry 的 AOP middleware 入口。
func Middleware(t *Telemetry) aop.MiddlewareFunc { return t.Middleware() }

// Interceptor 返回 Telemetry 的 AOP interceptor 入口。
func Interceptor(t *Telemetry) aop.InterceptorFunc { return t.Interceptor() }
