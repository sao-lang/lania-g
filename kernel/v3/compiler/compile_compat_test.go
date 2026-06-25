package compiler

import (
	"strings"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

type compatTestPlugin struct {
	scannedCount *int
}

func (p compatTestPlugin) ID() string { return "compat-test" }

func (p compatTestPlugin) Protocol() runtime.Protocol { return runtime.Protocol("compat-test") }

func (p compatTestPlugin) Register(reg *registry.Registry) {}

func (p compatTestPlugin) Scan(moduleRef *module.ModuleRef, reg *registry.Registry) (any, error) {
	count := len(reg.ListDecl("compat-test", "items"))
	if p.scannedCount != nil {
		*p.scannedCount = count
	}
	return struct{}{}, nil
}

func (p compatTestPlugin) Compile(scan any, reg *registry.Registry, global registry.GlobalAOPRegistration) (*CompiledProtocol, error) {
	return &CompiledProtocol{
		Protocol:        p.Protocol(),
		Routes:          map[string]*runtime.Handler{},
		RouteContainers: map[string]*di.Container{},
		Install:         ProtocolInstaller(p.Protocol(), nil, map[string]*runtime.Handler{}),
	}, nil
}

func TestCompileCompat_UsesGlobalRegistry(t *testing.T) {
	registry.ResetGlobal()
	t.Cleanup(registry.ResetGlobal)

	root := module.CreateModule(nil, nil, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init root: %v", err)
	}
	moduleRef := module.NewModuleRef(root)
	registry.Global().RegisterDecl("compat-test", "items", "global-item")

	var scanned int
	compiled, err := CompileCompat(moduleRef, compatTestPlugin{scannedCount: &scanned})
	if err != nil {
		t.Fatalf("compile compat: %v", err)
	}
	if compiled == nil || compiled.Diagnostics == nil {
		t.Fatalf("expected compiled diagnostics")
	}
	if scanned != 1 {
		t.Fatalf("scanned count = %d, want 1", scanned)
	}
}

func TestCompile_RequiresExplicitRegistry(t *testing.T) {
	root := module.CreateModule(nil, nil, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init root: %v", err)
	}
	moduleRef := module.NewModuleRef(root)

	_, err := Compile(moduleRef, nil, compatTestPlugin{})
	if err == nil {
		t.Fatalf("expected missing registry error")
	}
	if !strings.Contains(err.Error(), "requires an explicit registry") {
		t.Fatalf("unexpected error: %v", err)
	}
}
