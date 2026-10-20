package http_test

// 本文件锁定 ServeHTTP 链路中「不能被性能优化改掉」的语义：
// Abort 语义、下游 handler 只执行一次、以及无 query string 时的绑定结果。

import . "github.com/sao-lang/lania-g/protocol/http/v3"

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	httpbinding "github.com/sao-lang/lania-g/protocol/http/v3/binding"
	httpprotocol "github.com/sao-lang/lania-g/protocol/http/v3/protocol"
)

type abortProbeCtrl struct{ calls int }

func (c *abortProbeCtrl) Handle() (any, error) {
	c.calls++
	return "handled", nil
}

type queryProbeCtrl struct{}

func (c *queryProbeCtrl) Handle(q httpbinding.Query[string]) (any, error) {
	return "q=" + q.Value, nil
}

// mountProbe 挂载一个只注册单条路由的 adapter，并返回其 runtime。
func mountProbe(t *testing.T, a *Adapter, method, path string, ctrl any) *runtime.Runtime {
	t.Helper()
	rt := runtime.NewRuntime()
	if err := a.Mount(&fakeHost{rt: rt, reg: registry.New()}); err != nil {
		t.Fatalf("mount: %v", err)
	}
	httpbinding.RegisterDefaults(rt)
	registerHandler(t, rt, method, path, ctrl, "Handle")
	return rt
}

// Abort 之后 handler 与下游 http.Handler 都不应执行（gin/echo 语义）。
func TestServeChain_AbortSkipsHandlerAndDownstream(t *testing.T) {
	ctrl := &abortProbeCtrl{}
	a := testAdapter()
	mountProbe(t, a, http.MethodGet, "/blocked", ctrl)

	downstream := 0
	a.WithNextHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { downstream++ }))
	a.UsePreRouting(func(ctx *httpbinding.HttpContext) error {
		ctx.Abort()
		return ctx.Next()
	})

	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://example.com/blocked", nil))

	if ctrl.calls != 0 {
		t.Fatalf("handler calls = %d, want 0", ctrl.calls)
	}
	if downstream != 0 {
		t.Fatalf("downstream calls = %d, want 0", downstream)
	}
}

// 没有 preRouting 中间件时，请求仍必须正常进入 handler。
func TestServeChain_NoPreRoutingStillExecutesHandler(t *testing.T) {
	ctrl := &abortProbeCtrl{}
	a := testAdapter()
	mountProbe(t, a, http.MethodGet, "/plain", ctrl)

	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://example.com/plain", nil))

	if ctrl.calls != 1 {
		t.Fatalf("handler calls = %d, want 1", ctrl.calls)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

// registerQueryProbe 注册一个使用 Query 绑定（binding name = q）的路由。
func registerQueryProbe(t *testing.T, rt *runtime.Runtime, path string, ctrl any) {
	t.Helper()
	h, err := runtime.NewHandler(ctrl, "Handle")
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	h.Meta.RouteKey = runtime.BuildRouteKey(httpprotocol.Protocol, http.MethodGet, path)
	h.Meta.Protocol = httpprotocol.Protocol
	h.Meta.ParamPlans = []runtime.ParamPlan{{
		Index:       0,
		Type:        reflect.TypeFor[httpbinding.Query[string]](),
		BindingName: "q",
	}}
	rt.GetRouter().Register(h.Meta.RouteKey, h)
}

// 没有 query string 时，Query 绑定应得到零值而不是报错。
func TestServeChain_QueryBindingWithoutQueryString(t *testing.T) {
	a := testAdapter()
	rt := runtime.NewRuntime()
	if err := a.Mount(&fakeHost{rt: rt, reg: registry.New()}); err != nil {
		t.Fatalf("mount: %v", err)
	}
	httpbinding.RegisterDefaults(rt)
	registerQueryProbe(t, rt, "/q", &queryProbeCtrl{})

	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://example.com/q", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "q=") {
		t.Fatalf("body = %q, want it to contain %q", body, "q=")
	}
}

// 有 query string 时绑定仍然生效（避免短路条件写反）。
func TestServeChain_QueryBindingWithQueryString(t *testing.T) {
	a := testAdapter()
	rt := runtime.NewRuntime()
	if err := a.Mount(&fakeHost{rt: rt, reg: registry.New()}); err != nil {
		t.Fatalf("mount: %v", err)
	}
	httpbinding.RegisterDefaults(rt)
	registerQueryProbe(t, rt, "/q", &queryProbeCtrl{})

	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://example.com/q?q=hello", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "q=hello") {
		t.Fatalf("body = %q, want it to contain %q", body, "q=hello")
	}
}
