package resilience

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

type retryHandler struct{ calls int }

func (h *retryHandler) MaybeFail() (string, error) {
	h.calls++
	if h.calls == 1 {
		return "", fmt.Errorf("first failure")
	}
	return "ok", nil
}

type onceHandler struct{ calls int }

func (h *onceHandler) Handle() string {
	h.calls++
	return "payload"
}

type failHandler struct{ calls int }

func (h *failHandler) AlwaysFail() (string, error) {
	h.calls++
	return "", fmt.Errorf("boom")
}

func TestRetryAndCircuitBreaker(t *testing.T) {
	svc, err := New(Config{
		Retry: RetryConfig{Enabled: true, MaxAttempts: 2},
		Circuit: CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 1,
			OpenTimeout:      time.Minute,
		},
		Timeout:     TimeoutConfig{},
		Idempotency: IdempotencyConfig{},
		RateLimit:   RateLimitConfig{},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	okHandler := &retryHandler{}
	h1, _ := runtime.NewHandler(okHandler, "MaybeFail")
	h1.Meta.RouteKey = runtime.BuildRouteKey("http", "GET", "/retry")
	h1.Meta.Protocol = "http"

	rt := runtime.NewRuntime()
	rt.GetRouter().Register(h1.Meta.RouteKey, h1)
	rt.UseGlobalInterceptors(Interceptor(svc))

	ctx := runtime.NewHandlerContext("http")
	ctx.Request.Method = "GET"
	ctx.Request.Path = "/retry"
	out, err := rt.Execute(ctx)
	if err != nil || out.(string) != "ok" || okHandler.calls != 2 {
		t.Fatalf("retry out=%v err=%v calls=%d", out, err, okHandler.calls)
	}

	failSvc, _ := New(Config{
		Retry: RetryConfig{Enabled: false},
		Circuit: CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 1,
			OpenTimeout:      time.Minute,
		},
		Timeout:     TimeoutConfig{},
		Idempotency: IdempotencyConfig{},
		RateLimit:   RateLimitConfig{},
	})
	fail := &failHandler{}
	h2, _ := runtime.NewHandler(fail, "AlwaysFail")
	h2.Meta.RouteKey = runtime.BuildRouteKey("http", "GET", "/fail")
	h2.Meta.Protocol = "http"
	rt2 := runtime.NewRuntime()
	rt2.GetRouter().Register(h2.Meta.RouteKey, h2)
	rt2.UseGlobalInterceptors(Interceptor(failSvc))

	ctx2 := runtime.NewHandlerContext("http")
	ctx2.Request.Method = "GET"
	ctx2.Request.Path = "/fail"
	if _, err := rt2.Execute(ctx2); err == nil {
		t.Fatalf("first failure should error")
	}
	if _, err := rt2.Execute(ctx2); err == nil || !strings.Contains(err.Error(), "circuit breaker is open") {
		t.Fatalf("second failure err=%v", err)
	}
}

func TestRateLimitAndIdempotency(t *testing.T) {
	svc, err := New(Config{
		RateLimit:   RateLimitConfig{Enabled: true, Limit: 1, Window: time.Minute, Header: "X-RateLimit-Key"},
		Idempotency: IdempotencyConfig{Enabled: true, Header: "Idempotency-Key", TTL: time.Minute},
		Retry:       RetryConfig{Enabled: false},
		Timeout:     TimeoutConfig{},
		Circuit:     CircuitBreakerConfig{},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	handler := &onceHandler{}
	h, _ := runtime.NewHandler(handler, "Handle")
	h.Meta.RouteKey = runtime.BuildRouteKey("http", "POST", "/create")
	h.Meta.Protocol = "http"
	rt := runtime.NewRuntime()
	rt.GetRouter().Register(h.Meta.RouteKey, h)
	rt.UseGlobalMiddleware(Middleware(svc))
	rt.UseGlobalInterceptors(Interceptor(svc))

	first := runtime.NewHandlerContext("http")
	first.Request.Method = "POST"
	first.Request.Path = "/create"
	first.Request.Headers["Idempotency-Key"] = "idem-1"
	first.Request.Headers["X-RateLimit-Key"] = "user-1"
	if out, err := rt.Execute(first); err != nil || out.(string) != "payload" {
		t.Fatalf("first execute out=%v err=%v", out, err)
	}

	second := runtime.NewHandlerContext("http")
	second.Request.Method = "POST"
	second.Request.Path = "/create"
	second.Request.Headers["Idempotency-Key"] = "idem-1"
	second.Request.Headers["X-RateLimit-Key"] = "user-2"
	if out, err := rt.Execute(second); err != nil || out.(string) != "payload" || second.Response.Body != "payload" {
		t.Fatalf("second execute out=%v body=%v err=%v", out, second.Response.Body, err)
	}
	if handler.calls != 1 {
		t.Fatalf("idempotency should replay response, calls=%d", handler.calls)
	}

	limited := runtime.NewHandlerContext("http")
	limited.Request.Method = "POST"
	limited.Request.Path = "/create"
	limited.Request.Headers["X-RateLimit-Key"] = "same-user"
	if _, err := rt.Execute(limited); err != nil {
		t.Fatalf("first limited request: %v", err)
	}
	limited2 := runtime.NewHandlerContext("http")
	limited2.Request.Method = "POST"
	limited2.Request.Path = "/create"
	limited2.Request.Headers["X-RateLimit-Key"] = "same-user"
	if _, err := rt.Execute(limited2); err == nil || !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Fatalf("rate limit err=%v", err)
	}
}
