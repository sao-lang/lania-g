package application

import (
	"reflect"
	"testing"

	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

func newHTTPTestModule() module.Module {
	return httpadapter.Module(httpadapter.Config{})
}

type hotLoadBaseController struct{}

func (c *hotLoadBaseController) Ping() (string, error) {
	return "base", nil
}

func (c *hotLoadBaseController) Register(r *httpadapter.Routes) {
	r.Prefix("/base").Get("/ping", c.Ping)
}

type hotLoadExtraController struct{}

func (c *hotLoadExtraController) Ping() (string, error) {
	return "extra", nil
}

func (c *hotLoadExtraController) Register(r *httpadapter.Routes) {
	r.Prefix("/extra").Get("/ping", c.Ping)
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
		BaseModule: module.New(module.Metadata{
			Imports:     []module.Module{newHTTPTestModule()},
			Controllers: []any{ctrl},
		}),
	}
}

type hotLoadExtraModule struct {
	*module.BaseModule
}

func newHotLoadExtraModule(ctrl *hotLoadExtraController) module.Module {
	return &hotLoadExtraModule{
		BaseModule: module.New(module.Metadata{
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
		BaseModule: module.New(module.Metadata{
			Providers: []*di.Provider{p},
		}),
	}, nil
}

func TestHotLoad_RecompilesRuntimeWithNewModule(t *testing.T) {
	baseCtrl := &hotLoadBaseController{}
	extraCtrl := &hotLoadExtraController{}

	root := newHotLoadRootModule(baseCtrl)
	app, err := New(root, Options{})
	if err != nil {
		t.Fatalf("new application: %v", err)
	}

	if err := app.ensureCompiled(); err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got := executeTestHTTPPing(t, app, "/base/ping"); got != "base" {
		t.Fatalf("base response = %q, want %q", got, "base")
	}

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
	app, err := New(
		module.New(module.Metadata{Imports: []module.Module{newHTTPTestModule()}}),
		Options{},
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
