package runtime_test

// 锁定 Router 匹配语义与 routeKey 字符串格式：
// - 静态路由必须优先，且命中静态路由时不应再咨询协议 matcher
// - 静态未命中时才回退 matcher（动态路由能力不退化）
// - routeKey 格式是诊断/启动报告的输出契约，不能在优化中改变

import (
	"errors"
	"testing"

	. "github.com/sao-lang/lania-g/kernel/v3/runtime"
)

type routeCtrl struct{}

func (c *routeCtrl) Handle() (any, error) { return nil, nil }

func newRouteHandler(t *testing.T, routeKey string) *Handler {
	t.Helper()
	h, err := NewHandler(&routeCtrl{}, "Handle")
	if err != nil {
		t.Fatalf("new handler: %v", err)
	}
	h.Meta.RouteKey = routeKey
	return h
}

type countingMatcher struct {
	hits   int
	h      *Handler
	params map[string]string
}

func (m *countingMatcher) Match(ctx *HandlerContext) (*Handler, map[string]string) {
	m.hits++
	return m.h, m.params
}

func matchContext(method, path string) *HandlerContext {
	return &HandlerContext{
		Protocol: Protocol("http"),
		Request:  &Request{Method: method, Path: path},
	}
}

func TestRouterMatchesStaticRouteWithoutContributingParams(t *testing.T) {
	router := NewRouter()
	want := newRouteHandler(t, "http:GET:/users/1")
	router.Register("http:GET:/users/1", want)

	got, params, err := router.Match(matchContext("GET", "/users/1"))
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if got != want {
		t.Fatalf("handler = %v, want registered handler", got)
	}
	if params != nil {
		t.Fatalf("params = %v, want nil", params)
	}
}

func TestRouterDoesNotConsultMatcherWhenStaticRouteHits(t *testing.T) {
	router := NewRouter()
	matcher := &countingMatcher{}
	router.SetMatcher(Protocol("http"), matcher)
	router.Register("http:GET:/users/me", newRouteHandler(t, "http:GET:/users/me"))

	if _, _, err := router.Match(matchContext("GET", "/users/me")); err != nil {
		t.Fatalf("match: %v", err)
	}
	if matcher.hits != 0 {
		t.Fatalf("matcher hits = %d, want 0（静态路由命中后不应再咨询 matcher）", matcher.hits)
	}
}

func TestRouterFallsBackToMatcherForDynamicRoutes(t *testing.T) {
	router := NewRouter()
	dynamic := newRouteHandler(t, "http:GET:/users/:id")
	matcher := &countingMatcher{h: dynamic, params: map[string]string{"id": "42"}}
	router.SetMatcher(Protocol("http"), matcher)

	got, params, err := router.Match(matchContext("GET", "/users/42"))
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if got != dynamic {
		t.Fatalf("handler = %v, want matcher handler", got)
	}
	if params["id"] != "42" {
		t.Fatalf("params = %v, want id=42", params)
	}
	if matcher.hits != 1 {
		t.Fatalf("matcher hits = %d, want 1", matcher.hits)
	}
}

func TestRouterReportsRouteNotFound(t *testing.T) {
	router := NewRouter()
	router.Register("http:GET:/users", newRouteHandler(t, "http:GET:/users"))

	_, _, err := router.Match(matchContext("GET", "/missing"))
	if !errors.Is(err, ErrRouteNotFound) {
		t.Fatalf("err = %v, want ErrRouteNotFound", err)
	}
}

func TestRouterKeepsRouteKeyFormat(t *testing.T) {
	// 启动报告与诊断输出依赖该格式（protocol:method:path），优化不得改变。
	if got := BuildRouteKey(Protocol("http"), "GET", "/users/1"); got != "http:GET:/users/1" {
		t.Fatalf("routeKey = %q, want %q", got, "http:GET:/users/1")
	}
}
