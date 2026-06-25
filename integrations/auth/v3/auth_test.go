package auth

import (
	"reflect"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"github.com/sao-lang/lania-g/application/v3/factory"
	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

func TestAuthenticateJWTAndBindings(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRoot(Config{
		Name:      "admin",
		JWTSecret: "secret",
		APIKeys: map[string]Principal{
			"api-key": {Subject: "api-user", Roles: []string{"writer"}},
		},
		Sessions: map[string]Principal{
			"session-1": {Subject: "session-user", Roles: []string{"reader"}},
		},
	})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(interface{ SetRegistry(*registry.Registry) }).SetRegistry(registry.Global())
	if initErr := mod.Init(); initErr != nil {
		t.Fatalf("init: %v", initErr)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":       "user-1",
		"roles":     []string{"admin", "ops"},
		"tenant_id": "tenant-a",
	})
	signed, err := token.SignedString([]byte("secret"))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	ctx := runtime.NewHandlerContext("http")
	ctx.Container = mod.Container().NewChild()
	ctx.Request.Headers["Authorization"] = "Bearer " + signed

	serviceAny, err := mod.Container().Get(reflect.TypeFor[*Service]())
	if err != nil {
		t.Fatalf("get service: %v", err)
	}
	service := serviceAny.(*Service)
	principal, err := service.Authenticate(ctx)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if principal.Subject != "user-1" || principal.TenantID != "tenant-a" {
		t.Fatalf("principal = %+v", principal)
	}

	Middleware(service)(&aop.ExecutionContext{HandlerContext: ctx}, func() error { return nil })
	if Current(ctx) == nil || CurrentTenant(ctx) != "tenant-a" {
		t.Fatalf("principal not applied to context")
	}

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}

	value, err := br.Resolve(ctx, nil, reflect.TypeFor[InjectPrincipal](), 0)
	if err != nil {
		t.Fatalf("resolve principal: %v", err)
	}
	if value.(InjectPrincipal).Principal.Subject != "user-1" {
		t.Fatalf("resolved principal mismatch")
	}
	tenant, err := br.Resolve(ctx, nil, reflect.TypeFor[InjectTenant](), 1)
	if err != nil {
		t.Fatalf("resolve tenant: %v", err)
	}
	if tenant.(InjectTenant).Value != "tenant-a" {
		t.Fatalf("tenant mismatch")
	}
}

func TestAPIKeySessionAndGuards(t *testing.T) {
	service, err := New(Config{
		APIKeys: map[string]Principal{
			"api-key": {Subject: "api-user", Roles: []string{"writer"}},
		},
		Sessions: map[string]Principal{
			"session-1": {Subject: "session-user", Roles: []string{"reader"}, TenantID: "tenant-s"},
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	apiCtx := runtime.NewHandlerContext("http")
	apiCtx.Request.Headers["X-API-Key"] = "api-key"
	principal, err := service.Authenticate(apiCtx)
	if err != nil || principal.AuthType != "apikey" {
		t.Fatalf("api key auth: %+v %v", principal, err)
	}

	sessionCtx := runtime.NewHandlerContext("http")
	sessionCtx.Request.Headers["X-Session-Id"] = "session-1"
	if _, authErr := service.Authenticate(sessionCtx); authErr != nil {
		t.Fatalf("session auth: %v", authErr)
	}
	Middleware(service)(&aop.ExecutionContext{HandlerContext: sessionCtx}, func() error { return nil })

	guard := RequireRoles(service, "reader")
	ok, err := guard(&aop.ExecutionContext{HandlerContext: sessionCtx})
	if err != nil || !ok {
		t.Fatalf("guard result=%v err=%v", ok, err)
	}
	tenantGuard := RequireTenant(service)
	ok, err = tenantGuard(&aop.ExecutionContext{HandlerContext: sessionCtx})
	if err != nil || !ok {
		t.Fatalf("tenant guard result=%v err=%v", ok, err)
	}
}

func TestInstallHelpers(t *testing.T) {
	service, _ := New(Config{})
	app := &installTarget{}
	Install(app, service)
	if app.middlewares != 1 {
		t.Fatalf("install middlewares=%d", app.middlewares)
	}

	f := factory.New()
	if err := InstallOnFactory(f, service); err != nil {
		t.Fatalf("install on factory: %v", err)
	}
}

type installTarget struct{ middlewares int }

func (t *installTarget) UseGlobalMiddlewares(items ...aop.MiddlewareFunc) {
	t.middlewares += len(items)
}

func TestForRoot_InitRequiresExplicitRegistry(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRoot(Config{})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	if err := mod.Init(); err == nil {
		t.Fatalf("expected missing registry error")
	} else if !strings.Contains(err.Error(), "requires an explicit registry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestForRootCompat_InitRoutesToCompatFallbackSource(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRootCompat(Config{})
	if err != nil {
		t.Fatalf("for root compat: %v", err)
	}
	if err := mod.Init(); err != nil {
		t.Fatalf("init compat module: %v", err)
	}
	if got := registry.Global().SnapshotFallbackUsage()["integrations/auth.ForRootCompat"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}
