package compiler

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

func TestCompileError_ErrorAddsFriendlyOwnerSummaryForAmbiguousReceiver(t *testing.T) {
	err := &CompileError{
		Cause: NewOwnerResolutionError(
			"http",
			"http route GET /users/:id",
			OwnerKindReceiver,
			reflect.TypeOf(&ownerTestController{}),
			OwnerResolutionAmbiguous,
			[]ModuleOwner{{ModuleKey: "module.A"}, {ModuleKey: "module.B"}},
			nil,
			"",
		),
	}

	text := err.Error()
	for _, want := range []string{
		"owner diagnostics:",
		"receiver token",
		"matches multiple module owners",
		"module.A, module.B",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("compile error missing %q:\n%s", want, text)
		}
	}
}

func TestCompileError_ErrorAddsFriendlyOwnerSummaryForMissingReceiver(t *testing.T) {
	err := &CompileError{
		Cause: NewOwnerResolutionError(
			"scheduler",
			"scheduler job cleanup",
			OwnerKindReceiver,
			reflect.TypeOf(&ownerTestController{}),
			OwnerResolutionMissing,
			nil,
			nil,
			"",
		),
	}

	text := err.Error()
	for _, want := range []string{
		"owner diagnostics:",
		"receiver token",
		"is not attached to any module owner",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("compile error missing %q:\n%s", want, text)
		}
	}
}

func TestCompileDiagnostics_JSONIncludesCompatFallbackSummaries(t *testing.T) {
	diag := &CompileDiagnostics{
		GlobalAOP: AOPDiagnostics{Middlewares: 1},
		Protocols: map[runtime.Protocol]*ProtocolDiagnostics{
			runtime.Protocol("http"): {
				Protocol:         runtime.Protocol("http"),
				PluginID:         "http",
				DeclarationKinds: map[string]int{"routes": 1},
				DeclarationCount: 1,
				RouteCount:       1,
				RouteContainers:  1,
				OwnerModules: []ModuleRouteDiagnostics{
					{ModuleKey: "*example.UserModule", Routes: 1, RouteKeys: []string{"GET /users"}},
				},
			},
		},
		CompatFallbackCategories: []CompatFallbackCategorySummary{
			{Category: "packageDSL", Hits: 2, Sources: 1},
		},
		CompatFallbackSources: []CompatFallbackSourceSummary{
			{Source: "http.Controller", Hits: 2},
		},
	}

	data, err := json.Marshal(diag)
	if err != nil {
		t.Fatalf("marshal diagnostics: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"globalAOP",
		"middlewares",
		"protocols",
		"pluginId",
		"declarationKinds",
		"routeCount",
		"ownerModules",
		"moduleKey",
		"routeKeys",
		"compatFallbackCategories",
		"category",
		"hits",
		"sources",
		"packageDSL",
		"compatFallbackSources",
		"source",
		"http.Controller",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("diagnostics json missing %q: %s", want, text)
		}
	}

	if !diag.HasCompatFallbacks() {
		t.Fatalf("HasCompatFallbacks() = false, want true")
	}
}
