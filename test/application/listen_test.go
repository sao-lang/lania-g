package application_test

import (
	"reflect"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/module"
	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"

	. "github.com/sao-lang/lania-g/application/v3"
)

func httpModuleWith(addr string) module.Module {
	return httpadapter.Module(httpadapter.Config{Addr: addr})
}

func TestListenOverridesHTTPAddress(t *testing.T) {
	root := module.New(module.Metadata{Imports: []module.Module{httpModuleWith(":1111")}})
	app, err := New(root, Options{})
	if err != nil {
		t.Fatalf("new application: %v", err)
	}

	if err := app.Listen(":2222"); err != nil {
		t.Fatalf("listen: %v", err)
	}

	adapter, ok := resolveHTTPAdapter(t, app)
	if !ok {
		t.Fatalf("http adapter not found in container")
	}
	if got := adapter.Addr(); got != ":2222" {
		t.Fatalf("addr = %q, want :2222", got)
	}
}

func TestListenWithoutSharedListenerFails(t *testing.T) {
	root := module.New(module.Metadata{})
	app, err := New(root, Options{})
	if err != nil {
		t.Fatalf("new application: %v", err)
	}
	if err := app.Listen(":2222"); err == nil {
		t.Fatalf("expected error when no adapter requires shared listen")
	}
}

func TestListenRejectsEmptyAddr(t *testing.T) {
	root := module.New(module.Metadata{Imports: []module.Module{httpModuleWith(":1111")}})
	app, err := New(root, Options{})
	if err != nil {
		t.Fatalf("new application: %v", err)
	}
	if err := app.Listen("   "); err == nil {
		t.Fatalf("expected error for empty addr")
	}
}

func resolveHTTPAdapter(t *testing.T, app *Application) (*httpadapter.Adapter, bool) {
	t.Helper()
	if app.ModuleRef() == nil {
		return nil, false
	}
	container := app.ModuleRef().GetContainer()
	value, err := container.Get(reflect.TypeFor[*httpadapter.Adapter]())
	if err != nil {
		return nil, false
	}
	adapter, ok := value.(*httpadapter.Adapter)
	return adapter, ok
}
