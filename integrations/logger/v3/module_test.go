package logger

import (
	"bytes"
	stdctx "context"
	"reflect"
	"strings"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

type nopWriter struct{ bytes.Buffer }

func (w *nopWriter) Close() error { return nil }

func TestForRoot_RegistersLoggerFactoryAndConfig(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	writer := &nopWriter{}
	mod, err := ForRoot(Config{
		Level:  DebugLevel,
		Format: "json",
		Writer: writer,
	})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(interface{ SetRegistry(*registry.Registry) }).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	loggerToken := reflect.TypeFor[Logger]()
	factoryToken := reflect.TypeFor[Factory]()

	loggerAny, err := mod.Container().Get(loggerToken)
	if err != nil {
		t.Fatalf("get logger: %v", err)
	}
	lg := loggerAny.(Logger)
	lg.Info("hello", String("k", "v"))
	if !strings.Contains(writer.String(), "hello") {
		t.Fatalf("writer=%s", writer.String())
	}

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}
	ctx := runtime.NewHandlerContext("test")
	ctx.WithContext(stdctx.Background())
	ctx.Container = mod.Container().NewChild()

	value, err := br.Resolve(ctx, nil, loggerToken, 0)
	if err != nil {
		t.Fatalf("resolve logger: %v", err)
	}
	if value.(Logger) == nil {
		t.Fatalf("resolved logger nil")
	}

	factoryAny, err := br.Resolve(ctx, nil, factoryToken, 1)
	if err != nil {
		t.Fatalf("resolve factory: %v", err)
	}
	derived, err := factoryAny.(Factory).New(Config{Writer: &nopWriter{}})
	if err != nil || derived == nil {
		t.Fatalf("derived logger: %v", err)
	}
}

type auditLogger struct{}

func (auditLogger) LoggerName() string { return "audit" }

func TestBindings_ResolveNamedLoggerRef(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	writer := &nopWriter{}
	mod, err := ForRoot(Config{
		Name:   "audit",
		Level:  InfoLevel,
		Writer: writer,
	})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(interface{ SetRegistry(*registry.Registry) }).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}
	ctx := runtime.NewHandlerContext("test")
	ctx.WithContext(stdctx.Background())
	ctx.Container = mod.Container().NewChild()

	refType := reflect.TypeOf(LoggerRef[auditLogger]{})
	handler := &runtime.Handler{
		Meta: &runtime.HandlerMeta{ParamPlans: []runtime.ParamPlan{{Index: 0, Type: refType}}},
	}
	value, err := br.Resolve(ctx, handler, refType, 0)
	if err != nil {
		t.Fatalf("resolve logger ref: %v", err)
	}
	ref := value.(LoggerRef[auditLogger])
	ref.Info("audit-hit")
	if !strings.Contains(writer.String(), "audit-hit") {
		t.Fatalf("writer=%s", writer.String())
	}
}

func TestRegisterBindings_NilRoutesToCompatFallbackSource(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	RegisterBindings(nil)

	if got := registry.Global().SnapshotFallbackUsage()["integrations/logger.RegisterBindingsCompat"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
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
	if got := registry.Global().SnapshotFallbackUsage()["integrations/logger.ForRootCompat"]; got != 1 {
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
