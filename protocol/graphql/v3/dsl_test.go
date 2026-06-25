package graphql

import (
	"strings"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

type invalidResolver struct{}

func (r *invalidResolver) User() string { return "ok" }

func TestResolverBuilder_BuildE_RejectsUnsupportedSubscription(t *testing.T) {
	api := NewAPI(registry.New())
	resolver := &invalidResolver{}

	_, err := api.Resolver("User", resolver).Subscription("events", resolver.User).BuildE()
	if err == nil {
		t.Fatalf("BuildE: want error")
	}
	if !strings.Contains(err.Error(), "not supported yet") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildCompiledSchema_PrefersExplicitRegistryAndCompatGlobal(t *testing.T) {
	registry.ResetGlobal()
	t.Cleanup(registry.ResetGlobal)

	instanceReg := registry.New()
	resolver := &invalidResolver{}
	NewAPI(instanceReg).Resolver("User", resolver).Query("user", resolver.User).Build()
	NewAPI(registry.Global()).Resolver("GlobalUser", resolver).Query("user", resolver.User).Build()

	instanceSchema, err := buildCompiledSchemaFromRegistry(instanceReg, nil)
	if err != nil {
		t.Fatalf("build instance schema: %v", err)
	}
	if instanceSchema.field("Query", "user") == nil {
		t.Fatalf("expected instance schema query field")
	}
	if instanceSchema.object("User") == nil {
		t.Fatalf("expected instance schema resolver object")
	}
	if instanceSchema.object("GlobalUser") != nil {
		t.Fatalf("did not expect global resolver object in instance schema")
	}

	compatSchema, err := buildCompiledSchemaCompat(nil)
	if err != nil {
		t.Fatalf("build compat schema: %v", err)
	}
	if compatSchema.object("GlobalUser") == nil {
		t.Fatalf("expected compat schema resolver object")
	}
	if compatSchema.object("User") != nil {
		t.Fatalf("did not expect instance resolver object in compat schema")
	}
}

func TestBuildCompiledSchemaFromRegistry_RequiresExplicitRegistry(t *testing.T) {
	if _, err := buildCompiledSchemaFromRegistry(nil, nil); err == nil {
		t.Fatalf("expected missing registry error")
	} else if !strings.Contains(err.Error(), "requires registry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewAPI_NilRoutesToCompatFallbackSource(t *testing.T) {
	registry.ResetGlobal()
	t.Cleanup(registry.ResetGlobal)

	resolver := &invalidResolver{}
	NewAPI(nil).Resolver("User", resolver).Query("user", resolver.User).Build()

	if got := registry.Global().SnapshotFallbackUsage()["graphql.NewCompatAPI()"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}
