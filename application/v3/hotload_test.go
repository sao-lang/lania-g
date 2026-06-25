package application

import (
	"reflect"
	"testing"

	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

type hotLoadBaseController struct{}

func (c *hotLoadBaseController) Ping() (string, error) {
	return "base", nil
}

type hotLoadExtraController struct{}

func (c *hotLoadExtraController) Ping() (string, error) {
	return "extra", nil
}

type hotLoadBootstrapProvider struct {
	bootstrapped bool
}

func (p *hotLoadBootstrapProvider) OnApplicationBootstrap() error {
	p.bootstrapped = true
	return nil
}

type hotLoadRootModule struct {
	*module.BaseModule
}

func newHotLoadRootModule(ctrl *hotLoadBaseController) module.Module {
	return &hotLoadRootModule{
		BaseModule: module.NewBaseModule(&module.ModuleMetadata{
			Controllers: []any{ctrl},
		}),
	}
}

type hotLoadExtraModule struct {
	*module.BaseModule
}

func newHotLoadExtraModule(ctrl *hotLoadExtraController) module.Module {
	return &hotLoadExtraModule{
		BaseModule: module.NewBaseModule(&module.ModuleMetadata{
			Controllers: []any{ctrl},
		}),
	}
}

type hotLoadBootstrapModule struct {
	*module.BaseModule
}

func newHotLoadBootstrapModule(provider *hotLoadBootstrapProvider) (module.Module, error) {
	p, err := di.ProviderFromInstanceWithToken(reflect.TypeOf(provider), provider, di.Singleton)
	if err != nil {
		return nil, err
	}
	return &hotLoadBootstrapModule{
		BaseModule: module.NewBaseModule(&module.ModuleMetadata{
			Providers: []*di.Provider{p},
		}),
	}, nil
}

func TestHotLoad_RecompilesRuntimeWithNewModule(t *testing.T) {
	reg := registry.New()
	baseCtrl := &hotLoadBaseController{}
	extraCtrl := &hotLoadExtraController{}

	root := newHotLoadRootModule(baseCtrl)
	app, err := NewWithOptions(root, Options{Registry: reg}, httpadapter.New())
	if err != nil {
		t.Fatalf("new application: %v", err)
	}

	api := mustTestAdapterAPI[*httpadapter.API](t, app, "http")
	api.Controller("/base", baseCtrl).Get("/ping", baseCtrl.Ping).Build()

	if _, err := app.CompileDiagnostics(); err != nil {
		t.Fatalf("compile diagnostics: %v", err)
	}
	if got := executeTestHTTPPing(t, app, "/base/ping"); got != "base" {
		t.Fatalf("base response = %q, want %q", got, "base")
	}

	api.Controller("/extra", extraCtrl).Get("/ping", extraCtrl.Ping).Build()

	hotModule := newHotLoadExtraModule(extraCtrl)
	if err := app.HotLoad(hotModule); err != nil {
		t.Fatalf("hot load: %v", err)
	}

	if got := executeTestHTTPPing(t, app, "/base/ping"); got != "base" {
		t.Fatalf("base response after hot load = %q, want %q", got, "base")
	}
	if got := executeTestHTTPPing(t, app, "/extra/ping"); got != "extra" {
		t.Fatalf("extra response after hot load = %q, want %q", got, "extra")
	}
}

func TestHotLoad_BootstrapsNewModuleWhenApplicationAlreadyBootstrapped(t *testing.T) {
	app, err := NewWithOptions(
		module.CreateModule(nil, nil, nil, nil, nil),
		Options{Registry: registry.New()},
		httpadapter.New(),
	)
	if err != nil {
		t.Fatalf("new application: %v", err)
	}
	if err := app.bootstrapLifecycle(); err != nil {
		t.Fatalf("bootstrap lifecycle: %v", err)
	}

	provider := &hotLoadBootstrapProvider{}
	hotModule, err := newHotLoadBootstrapModule(provider)
	if err != nil {
		t.Fatalf("new hot-load bootstrap module: %v", err)
	}
	if err := app.HotLoad(hotModule); err != nil {
		t.Fatalf("hot load: %v", err)
	}
	if !provider.bootstrapped {
		t.Fatalf("expected hot-loaded provider to receive application bootstrap")
	}
}
