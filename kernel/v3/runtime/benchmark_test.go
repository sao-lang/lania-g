package runtime

import (
	"reflect"
	"testing"
)

type benchmarkService struct{}

// Ping 是基准测试用的最小 handler 方法：无入参、返回一个 string。
func (s *benchmarkService) Ping() string { return "pong" }

type benchmarkWrapper string

// BenchmarkExecutorExecuteStaticRoute 衡量“静态路由（精确匹配）+ 无参 handler”的整体执行开销。
//
// 该 benchmark 主要覆盖：
// - Router 精确匹配
// - Executor 参数解析（无参数时的最小路径）
// - Pipeline 默认链路与 Handler.Invoke
func BenchmarkExecutorExecuteStaticRoute(b *testing.B) {
	handler, err := NewHandler(&benchmarkService{}, "Ping")
	if err != nil {
		b.Fatalf("new handler: %v", err)
	}
	handler.Meta.RouteKey = BuildRouteKey(Protocol("bench"), "GET", "/ping")
	handler.Meta.Protocol = Protocol("bench")

	rt := NewRuntime()
	rt.GetRouter().Register(handler.Meta.RouteKey, handler)

	ctx := NewHandlerContext(Protocol("bench"))
	ctx.Request.Method = "GET"
	ctx.Request.Path = "/ping"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := rt.Execute(ctx)
		if err != nil {
			b.Fatalf("execute: %v", err)
		}
	}
}

// BenchmarkBindingRegistryFindCached 衡量 BindingRegistry.Find 在“命中缓存”情况下的查找开销。
//
// 该 benchmark 会先 warm up 缓存，再在循环中反复 Find 同一个 reflect.Type。
func BenchmarkBindingRegistryFindCached(b *testing.B) {
	reg := NewBindingRegistry()
	targetType := reflect.TypeOf(benchmarkWrapper(""))
	reg.RegisterFunc(func(t reflect.Type) bool {
		return t == targetType
	}, func(ctx *HandlerContext, paramType reflect.Type) (interface{}, error) {
		return benchmarkWrapper("ok"), nil
	})

	if _, _, ok := reg.Find(targetType); !ok {
		b.Fatalf("warm cache failed")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, ok := reg.Find(targetType); !ok {
			b.Fatalf("find cached failed")
		}
	}
}
