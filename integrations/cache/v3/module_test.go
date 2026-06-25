package cache

import (
	stdctx "context"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

func TestForRoot_RegistersCacheFactoryAndConfig(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRoot(Config{Type: Memory, Name: "default"})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(interface{ SetRegistry(*registry.Registry) }).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	defer mod.Destroy()

	cacheToken := reflect.TypeFor[Cache]()
	factoryToken := reflect.TypeFor[Factory]()

	cacheAny, err := mod.Container().Get(cacheToken)
	if err != nil {
		t.Fatalf("get cache: %v", err)
	}
	cache := cacheAny.(Cache)
	if err := cache.Set("foo", "bar"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got, _ := cache.GetString("foo"); got != "bar" {
		t.Fatalf("foo=%s", got)
	}

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}
	ctx := runtime.NewHandlerContext("test")
	ctx.WithContext(stdctx.Background())
	ctx.Container = mod.Container().NewChild()

	value, err := br.Resolve(ctx, nil, cacheToken, 0)
	if err != nil {
		t.Fatalf("resolve cache: %v", err)
	}
	if value.(Cache) != cache {
		t.Fatalf("resolved cache mismatch")
	}

	factoryValue, err := br.Resolve(ctx, nil, factoryToken, 1)
	if err != nil {
		t.Fatalf("resolve factory: %v", err)
	}
	factory := factoryValue.(Factory)
	named, err := factory.GetOrCreate("secondary", Config{Type: Memory, Name: "secondary"})
	if err != nil {
		t.Fatalf("named cache: %v", err)
	}
	if err := named.SetEx("k", "v", 50*time.Millisecond); err != nil {
		t.Fatalf("setex: %v", err)
	}
	if got, _ := named.GetString("k"); got != "v" {
		t.Fatalf("named value=%s", got)
	}
}

type analyticsCache struct{}

func (analyticsCache) CacheName() string { return "analytics" }

func TestBindings_ResolveNamedCacheRef(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRoot(Config{Type: Memory, Name: "analytics"})
	if err != nil {
		t.Fatalf("for root: %v", err)
	}
	mod.(interface{ SetRegistry(*registry.Registry) }).SetRegistry(registry.Global())
	if err := mod.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	defer mod.Destroy()

	br := runtime.NewBindingRegistry()
	for _, resolver := range registry.Global().GetBindings() {
		br.Register(resolver)
	}
	ctx := runtime.NewHandlerContext("test")
	ctx.WithContext(stdctx.Background())
	ctx.Container = mod.Container().NewChild()

	refType := reflect.TypeOf(CacheRef[analyticsCache]{})
	handler := &runtime.Handler{
		Meta: &runtime.HandlerMeta{
			ParamPlans: []runtime.ParamPlan{{Index: 0, Type: refType}},
		},
	}
	value, err := br.Resolve(ctx, handler, refType, 0)
	if err != nil {
		t.Fatalf("resolve named cache ref: %v", err)
	}
	ref := value.(CacheRef[analyticsCache])
	if err := ref.Set("named", "cache"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if got, _ := ref.GetString("named"); got != "cache" {
		t.Fatalf("named=%s", got)
	}
}

func TestRememberAndDecorate(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	var loads atomic.Int32
	value, err := Remember(cache, "remember:key", Policy{}, func() (string, error) {
		loads.Add(1)
		return "cached", nil
	})
	if err != nil || value != "cached" {
		t.Fatalf("remember value=%s err=%v", value, err)
	}
	value, err = Remember(cache, "remember:key", Policy{}, func() (string, error) {
		loads.Add(1)
		return "miss", nil
	})
	if err != nil || value != "cached" || loads.Load() != 1 {
		t.Fatalf("remember cached=%s loads=%d err=%v", value, loads.Load(), err)
	}

	var calls atomic.Int32
	fn := Decorate(func(id int) (string, error) {
		calls.Add(1)
		return "user", nil
	}, cache, DecoratorOptions{
		Key:    "user-by-id",
		Policy: Policy{TTL: time.Minute},
	}).(func(int) (string, error))

	first, err := fn(1)
	if err != nil || first != "user" {
		t.Fatalf("first=%s err=%v", first, err)
	}
	second, err := fn(1)
	if err != nil || second != "user" {
		t.Fatalf("second=%s err=%v", second, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

func TestTemplateKeyBuilderAndInvalidatePattern(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	builder := TemplateKeyBuilder("user:{0}:orders:{1}")
	key, err := builder([]reflect.Value{reflect.ValueOf("u1"), reflect.ValueOf(2)})
	if err != nil || key != "user:u1:orders:2" {
		t.Fatalf("key=%s err=%v", key, err)
	}
	if err := cache.Set("user:u1:orders:1", "a"); err != nil {
		t.Fatalf("set a: %v", err)
	}
	if err := cache.Set("user:u1:orders:2", "b"); err != nil {
		t.Fatalf("set b: %v", err)
	}
	if err := InvalidatePattern(cache, "user:u1:orders:*"); err != nil {
		t.Fatalf("invalidate pattern: %v", err)
	}
	if exists, _ := cache.Exists("user:u1:orders:1"); exists {
		t.Fatalf("pattern invalidate failed")
	}
}

func TestDecorateE_InvalidTarget(t *testing.T) {
	cache := NewMemoryCache()
	defer cache.Close()

	wrapped, err := DecorateE("not-a-function", cache, DecoratorOptions{})
	if err == nil {
		t.Fatalf("expected decorate error")
	}
	if wrapped != nil {
		t.Fatalf("wrapped should be nil")
	}
}

func TestForRoot_InitRequiresExplicitRegistry(t *testing.T) {
	registry.ResetGlobal()
	defer registry.ResetGlobal()

	mod, err := ForRoot(Config{Type: Memory, Name: "default"})
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

	mod, err := ForRootCompat(Config{Type: Memory, Name: "default"})
	if err != nil {
		t.Fatalf("for root compat: %v", err)
	}
	if err := mod.Init(); err != nil {
		t.Fatalf("init compat module: %v", err)
	}
	if got := registry.Global().SnapshotFallbackUsage()["integrations/cache.ForRootCompat"]; got != 1 {
		t.Fatalf("fallback usage = %d, want 1", got)
	}
}
