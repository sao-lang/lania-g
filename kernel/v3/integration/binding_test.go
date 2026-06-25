package integration

import (
	"reflect"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

func TestRegisterContainerBindings_NilRoutesToCompatFallbackSource(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	RegisterContainerBindings(nil, NewBindingEntry("sample", reflect.TypeOf("")))

	if got := registry.Global().SnapshotFallbackUsage()["core/integration.RegisterContainerBindingsCompat"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}
