package cache

import (
	"reflect"
	"sync/atomic"
	"testing"
	"time"

)

type analyticsCache struct{}

func (analyticsCache) CacheName() string { return "analytics" }

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

