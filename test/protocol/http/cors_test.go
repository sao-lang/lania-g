package http_test

import . "github.com/sao-lang/lania-g/protocol/http/v3"
import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

func newCorsAdapter(t *testing.T, cfg *CorsConfig) *Adapter {
	t.Helper()
	a := New(Config{CORS: cfg})
	if err := a.Mount(&fakeHost{rt: runtime.NewRuntime(), reg: registry.New()}); err != nil {
		t.Fatalf("mount: %v", err)
	}
	return a
}

func preflight(t *testing.T, a *Adapter, origin string, requestHeaders string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodOptions, "http://example.com/users", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", "GET")
	if requestHeaders != "" {
		req.Header.Set("Access-Control-Request-Headers", requestHeaders)
	}
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)
	return rec
}

func TestCORS_PreflightWildcard(t *testing.T) {
	a := newCorsAdapter(t, DefaultCorsConfig())
	rec := preflight(t, a, "https://app.example", "")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("allow-origin = %q", got)
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatalf("allow-methods should be present")
	}
	if !varyContains(rec.Header().Values("Vary"), "Origin") {
		t.Fatalf("Vary should contain Origin, got %v", rec.Header().Values("Vary"))
	}
}

func TestCORS_WildcardWithCredentialsEchoesOrigin(t *testing.T) {
	cfg := DefaultCorsConfig()
	cfg.AllowCredentials = true
	a := newCorsAdapter(t, cfg)

	rec := preflight(t, a, "https://app.example", "")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("allow-origin = %q, want echoed origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("allow-credentials = %q", got)
	}
}

func TestCORS_DoesNotEchoRequestHeadersByDefault(t *testing.T) {
	cfg := DefaultCorsConfig()
	cfg.AllowHeaders = nil
	a := newCorsAdapter(t, cfg)

	rec := preflight(t, a, "https://app.example", "x-custom, x-secret")
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "" {
		t.Fatalf("allow-headers = %q, want empty", got)
	}
}

func TestCORS_AllowAllHeadersOptIn(t *testing.T) {
	cfg := DefaultCorsConfig()
	cfg.AllowHeaders = nil
	cfg.AllowAllHeaders = true
	a := newCorsAdapter(t, cfg)

	rec := preflight(t, a, "https://app.example", "x-custom")
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "x-custom" {
		t.Fatalf("allow-headers = %q", got)
	}
}

func TestCORS_AllowOriginFunc(t *testing.T) {
	cfg := DefaultCorsConfig()
	cfg.AllowOrigins = nil
	cfg.AllowOriginFunc = func(origin string) bool {
		return strings.HasSuffix(origin, ".example.com")
	}
	a := newCorsAdapter(t, cfg)

	allowed := preflight(t, a, "https://api.example.com", "")
	if got := allowed.Header().Get("Access-Control-Allow-Origin"); got != "https://api.example.com" {
		t.Fatalf("allow-origin = %q", got)
	}

	// 未命中的 origin 仍然继续走路由（这里是 NotFound），但不应写 CORS 头。
	req := httptest.NewRequest(http.MethodGet, "http://example.com/users", nil)
	req.Header.Set("Origin", "https://evil.test")
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("allow-origin = %q, want empty", got)
	}
	if !varyContains(rec.Header().Values("Vary"), "Origin") {
		t.Fatalf("Vary should still contain Origin")
	}
}

func TestCORS_SimpleRequestAddsExposeHeaders(t *testing.T) {
	cfg := DefaultCorsConfig()
	cfg.ExposeHeaders = []string{"X-Request-Id"}
	a := newCorsAdapter(t, cfg)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/x", nil)
	req.Header.Set("Origin", "https://app.example")
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Expose-Headers"); got != "X-Request-Id" {
		t.Fatalf("expose-headers = %q", got)
	}
}

func varyContains(values []string, want string) bool {
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), want) {
				return true
			}
		}
	}
	return false
}
