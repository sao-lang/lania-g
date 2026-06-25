package http

import (
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

type testHTTPController struct{}

func (c *testHTTPController) List() error { return nil }

func TestControllerBuilder_BuildEOnScopeMisuse(t *testing.T) {
	reg := registry.New()
	ctrl := &testHTTPController{}
	builder := NewAPI(reg, nil).Controller("/users", ctrl)
	builder.Get("/", ctrl.List)
	builder.UseGuards("auth")

	if builder.Err() == nil {
		t.Fatalf("expected builder error")
	}
	defs, err := builder.BuildE()
	if err == nil {
		t.Fatalf("expected build error")
	}
	if defs != nil {
		t.Fatalf("defs should be nil")
	}
}

func TestNewAPI_NilRoutesToCompatFallbackSource(t *testing.T) {
	registry.ResetGlobal()
	t.Cleanup(registry.ResetGlobal)

	ctrl := &testHTTPController{}
	NewAPI(nil, nil).Controller("/users", ctrl).Get("/", ctrl.List).Build()

	if got := registry.Global().SnapshotFallbackUsage()["http.NewCompatAPI()"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}
