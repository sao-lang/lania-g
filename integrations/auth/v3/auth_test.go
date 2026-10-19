package auth

import (
	"testing"


	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

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

type installTarget struct{ middlewares int }

func (t *installTarget) UseGlobalMiddlewares(items ...aop.MiddlewareFunc) {
	t.middlewares += len(items)
}

