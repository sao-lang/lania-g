package http

import (
	"testing"

	coreregistry "github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

type mustBindPayload struct {
	UserID string `query:"user_id" required:"true"`
}

func TestBindInto_ReturnsErrorForMissingRequiredQuery(t *testing.T) {
	ctx := runtime.NewHandlerContext("http")
	var payload mustBindPayload

	err := BindInto(ctx, &payload)
	if err == nil {
		t.Fatalf("BindInto: want error")
	}
}

func TestMustBindInto_PanicsOnBindError(t *testing.T) {
	ctx := runtime.NewHandlerContext("http")
	var payload mustBindPayload

	defer func() {
		if recover() == nil {
			t.Fatalf("MustBindInto: want panic")
		}
	}()
	MustBindInto(ctx, &payload)
}

func TestRegisterDefaultsToRegistry_NilRoutesToCompatFallbackSource(t *testing.T) {
	coreregistry.ResetGlobal()
	defer coreregistry.ResetGlobal()

	RegisterDefaultsToRegistry(nil)

	if got := coreregistry.Global().SnapshotFallbackUsage()["binding/http.RegisterDefaultsCompat"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}
