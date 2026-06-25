// refs.go 定义 cache integration 的命名引用 wrapper 与 binding 注册辅助。
package cache

import (
	"fmt"
	"reflect"

	coredi "github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// CacheNamer 约定命名缓存引用的名称来源。
type CacheNamer interface {
	CacheName() string
}

// DefaultCache 用于标记“默认缓存实例”引用。
type DefaultCache struct{}

// CacheName 返回默认缓存实例的名称。
func (DefaultCache) CacheName() string { return "default" }

// InjectCache 表示注入默认缓存实例的包装类型。
type InjectCache struct {
	Cache
}

// CacheRef 用于按命名引用注入一个缓存实例。
type CacheRef[N any] struct {
	Cache
	_ *N
}

// CacheToken 返回某个命名缓存实例对应的 DI token。
func CacheToken(name string) string {
	if name == "" {
		name = "default"
	}
	return "cache:instance:" + name
}

// RegisterBindings 注册缓存引用相关的 binding。
func RegisterBindings(reg *registry.Registry) {
	if reg == nil {
		RegisterBindingsCompat()
		return
	}
	registerBindings(reg)
}

// RegisterBindingsCompat 显式保留“写入全局 registry”的兼容 binding 注册入口。
func RegisterBindingsCompat() {
	registerBindings(registry.GlobalWithUsage("integrations/cache.RegisterBindingsCompat"))
}

func registerBindings(reg *registry.Registry) {
	reg.RegisterBindings(runtime.NewBindingResolvers(
		registration("CacheInject", coreintegration.MatchNamedWrapper(packagePath(), "InjectCache"), resolveInjectCache),
		registration("CacheRef", coreintegration.MatchNamedWrapper(packagePath(), "CacheRef"), resolveCacheRef),
	)...)
}

func resolveInjectCache(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	cache, err := resolveNamedCache(ctx, "default")
	if err != nil {
		return nil, err
	}
	return coreintegration.WrapFirstField(desc.WrapperType, reflect.ValueOf(cache))
}

func resolveCacheRef(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	cache, err := resolveNamedCache(ctx, cacheNameFromWrapper(desc.WrapperType))
	if err != nil {
		return nil, err
	}
	return coreintegration.WrapFirstField(desc.WrapperType, reflect.ValueOf(cache))
}

func resolveNamedCache(ctx *runtime.HandlerContext, name string) (Cache, error) {
	if ctx == nil || ctx.Container == nil {
		return nil, fmt.Errorf("cache binding requires request container")
	}
	if value, err := ctx.Container.Get(CacheToken(name)); err == nil {
		if cache, ok := value.(Cache); ok {
			return cache, nil
		}
	}
	value, err := coredi.GetByType[Cache](ctx.Container)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func cacheNameFromWrapper(wrapperType reflect.Type) string {
	return coreintegration.ResolveMarkerName(wrapperType, "default", func(marker any) (string, bool) {
		namer, ok := marker.(CacheNamer)
		if !ok {
			return "", false
		}
		return namer.CacheName(), true
	})
}

func registration(name string, match func(reflect.Type) (runtime.WrapperDescriptor, bool), resolve func(*runtime.HandlerContext, runtime.WrapperDescriptor) (any, error)) runtime.BindingRegistration {
	return runtime.BindingRegistration{
		Name:    name,
		Match:   match,
		Resolve: resolve,
	}
}

func packagePath() string {
	return reflect.TypeFor[CacheNamer]().PkgPath()
}
