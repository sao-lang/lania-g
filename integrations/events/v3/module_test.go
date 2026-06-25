package events

import (
	stdctx "context"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

func TestForRoot_RegistersBusEmitterFactoryAndConfig(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRoot(Config{})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(interface{ SetRegistry(*registry.Registry) }).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	busToken := reflect.TypeFor[*Bus]()
	emitterToken := reflect.TypeFor[Emitter]()
	factoryToken := reflect.TypeFor[Factory]()

	busAny, err := mod.Container().Get(busToken)
	if err != nil {
		t.Fatalf("get bus: %v", err)
	}
	bus := busAny.(*Bus)

	var called atomic.Int32
	done := make(chan struct{}, 1)
	bus.On("user.created", func(ctx stdctx.Context, args ...interface{}) error {
		called.Add(1)
		done <- struct{}{}
		return nil
	})
	if err := bus.Emit(stdctx.Background(), "user.created", "u1"); err != nil {
		t.Fatalf("emit: %v", err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("handler not called")
	}
	if called.Load() != 1 {
		t.Fatalf("called=%d", called.Load())
	}

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}
	ctx := runtime.NewHandlerContext("test")
	ctx.WithContext(stdctx.Background())
	ctx.Container = mod.Container().NewChild()

	if _, err := br.Resolve(ctx, nil, busToken, 0); err != nil {
		t.Fatalf("resolve bus: %v", err)
	}
	if _, err := br.Resolve(ctx, nil, emitterToken, 1); err != nil {
		t.Fatalf("resolve emitter: %v", err)
	}
	factoryValue, err := br.Resolve(ctx, nil, factoryToken, 2)
	if err != nil {
		t.Fatalf("resolve factory: %v", err)
	}
	derived, err := factoryValue.(Factory).New(Config{})
	if err != nil || derived == nil {
		t.Fatalf("derived bus: %v", err)
	}
}

type eventBridgeService struct {
	count atomic.Int32
}

func (s *eventBridgeService) HandleUserCreated(ctx stdctx.Context, name string) error {
	_ = ctx
	if name != "alice" {
		return stdctx.Canceled
	}
	s.count.Add(1)
	return nil
}

func TestAttachRegisteredHandlers(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	svc := &eventBridgeService{}
	pSvc, _ := di.ProviderFromInstanceWithToken(reflect.TypeOf(svc), svc, di.Singleton)
	root := module.CreateModule(nil, []*di.Provider{pSvc}, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	ref := module.NewModuleRef(root)
	bus, err := New(Config{})
	if err != nil {
		t.Fatalf("new bus: %v", err)
	}
	RegisterOn(registry.Global(), "user.created", svc, svc.HandleUserCreated)
	if err := AttachRegisteredHandlers(bus, ref, registry.Global()); err != nil {
		t.Fatalf("attach handlers: %v", err)
	}
	if err := bus.Emit(stdctx.Background(), "user.created", "alice"); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if svc.count.Load() != 1 {
		t.Fatalf("count=%d", svc.count.Load())
	}
}

func TestAttachRegisteredHandlers_RequiresExplicitRegistry(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	svc := &eventBridgeService{}
	pSvc, _ := di.ProviderFromInstanceWithToken(reflect.TypeOf(svc), svc, di.Singleton)
	root := module.CreateModule(nil, []*di.Provider{pSvc}, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	ref := module.NewModuleRef(root)
	bus, err := New(Config{})
	if err != nil {
		t.Fatalf("new bus: %v", err)
	}
	RegisterOn(registry.Global(), "user.created", svc, svc.HandleUserCreated)

	if err := AttachRegisteredHandlers(bus, ref, nil); err == nil {
		t.Fatalf("expected missing registry error")
	} else if !strings.Contains(err.Error(), "requires an explicit registry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegisterOn_NilRoutesToCompatFallbackSource(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	svc := &eventBridgeService{}
	RegisterOnCompat("user.created", svc, svc.HandleUserCreated)

	if got := registry.Global().SnapshotFallbackUsage()["integrations/events.RegisterOnCompat"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}

func TestRegisterHandlers_NilRoutesToCompatFallbackSource(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	RegisterHandlersCompat(&HandlerDefinition{Event: "user.created", HandlerName: "HandleUserCreated"})

	if got := registry.Global().SnapshotFallbackUsage()["integrations/events.RegisterHandlersCompat"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}

func TestForRoot_InitRequiresExplicitRegistry(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRoot(Config{})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	if err := mod.Init(); err == nil {
		t.Fatalf("expected missing registry error")
	} else if !strings.Contains(err.Error(), "requires an explicit registry") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestForRootCompat_InitRoutesToCompatFallbackSource(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRootCompat(Config{})
	if err != nil {
		t.Fatalf("for root compat: %v", err)
	}
	if err := mod.Init(); err != nil {
		t.Fatalf("init compat module: %v", err)
	}
	if got := registry.Global().SnapshotFallbackUsage()["integrations/events.ForRootCompat"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}

func TestAttachRegisteredHandlersCompat(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	svc := &eventBridgeService{}
	pSvc, _ := di.ProviderFromInstanceWithToken(reflect.TypeOf(svc), svc, di.Singleton)
	root := module.CreateModule(nil, []*di.Provider{pSvc}, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	ref := module.NewModuleRef(root)
	bus, err := New(Config{})
	if err != nil {
		t.Fatalf("new bus: %v", err)
	}
	RegisterOn(registry.Global(), "user.created", svc, svc.HandleUserCreated)
	if err := AttachRegisteredHandlersCompat(bus, ref); err != nil {
		t.Fatalf("attach compat handlers: %v", err)
	}
	if err := bus.Emit(stdctx.Background(), "user.created", "alice"); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if svc.count.Load() != 1 {
		t.Fatalf("count=%d", svc.count.Load())
	}
}

func TestLifecycleHook_AttachesOnApplicationBootstrap(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	svc := &eventBridgeService{}
	bus, err := New(Config{})
	if err != nil {
		t.Fatalf("new bus: %v", err)
	}
	RegisterOn(registry.Global(), "user.created", svc, svc.HandleUserCreated)
	hook := NewLifecycleHook(bus, registry.Global())
	if err := hook.OnApplicationBootstrap(); err != nil {
		t.Fatalf("bootstrap hook: %v", err)
	}
	if err := bus.Emit(stdctx.Background(), "user.created", "alice"); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if svc.count.Load() != 1 {
		t.Fatalf("count=%d", svc.count.Load())
	}
}

func TestLifecycleHookCompat_AttachesOnApplicationBootstrap(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	svc := &eventBridgeService{}
	bus, err := New(Config{})
	if err != nil {
		t.Fatalf("new bus: %v", err)
	}
	RegisterOn(registry.Global(), "user.created", svc, svc.HandleUserCreated)
	hook := NewLifecycleHookCompat(bus)
	if err := hook.OnApplicationBootstrap(); err != nil {
		t.Fatalf("bootstrap hook compat: %v", err)
	}
	if err := bus.Emit(stdctx.Background(), "user.created", "alice"); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if svc.count.Load() != 1 {
		t.Fatalf("count=%d", svc.count.Load())
	}
}

func TestLifecycleHook_UsesModuleRefReceiver(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	original := &eventBridgeService{}
	resolved := &eventBridgeService{}
	bus, err := New(Config{})
	if err != nil {
		t.Fatalf("new bus: %v", err)
	}
	svcProvider, _ := di.ProviderFromInstanceWithToken(reflect.TypeOf(resolved), resolved, di.Singleton)
	root := module.CreateModule(nil, []*di.Provider{svcProvider}, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init root: %v", err)
	}
	ref := module.NewModuleRef(root)

	RegisterOn(registry.Global(), "user.created", original, original.HandleUserCreated)
	hook := NewLifecycleHook(bus, registry.Global())
	if setter, ok := hook.(interface{ SetModuleRef(*module.ModuleRef) }); ok {
		setter.SetModuleRef(ref)
	}
	if err := hook.OnApplicationBootstrap(); err != nil {
		t.Fatalf("bootstrap hook: %v", err)
	}
	if err := bus.Emit(stdctx.Background(), "user.created", "alice"); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if resolved.count.Load() != 1 {
		t.Fatalf("resolved count=%d", resolved.count.Load())
	}
	if original.count.Load() != 0 {
		t.Fatalf("original count=%d", original.count.Load())
	}
}
