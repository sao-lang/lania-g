// binding.go 实现 mongodb integration 暴露给 handler 的 binding 包装与 resolver。
package mongodb

import (
	"fmt"
	"reflect"
	"strings"

	"go.mongodb.org/mongo-driver/mongo"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// RegisterBindings 注册 MongoDB 相关的命名注入 wrapper。
func RegisterBindings(reg *registry.Registry) {
	if reg == nil {
		RegisterBindingsCompat()
		return
	}
	registerBindings(reg)
}

// RegisterBindingsCompat 显式保留“写入全局 registry”的兼容 binding 注册入口。
func RegisterBindingsCompat() {
	registerBindings(registry.GlobalWithUsage("integrations/mongodb.RegisterBindingsCompat"))
}

func registerBindings(reg *registry.Registry) {
	reg.RegisterBindings(
		&resolver{name: "MongoInjectDatabase", match: matchWrapper("InjectDatabase"), resolve: resolveInjectDatabase},
		&resolver{name: "MongoInjectClient", match: matchWrapper("InjectClient"), resolve: resolveInjectClient},
		&resolver{name: "MongoDatabaseRef", match: matchWrapper("DatabaseRef"), resolve: resolveDatabaseRef},
	)
}

type resolver struct {
	name    string
	match   func(reflect.Type) (runtime.WrapperDescriptor, bool)
	resolve func(*runtime.HandlerContext, runtime.WrapperDescriptor) (any, error)
}

// Name 返回 resolver 名称。
func (r *resolver) Name() string { return r.name }

// AllowedProtocols 返回允许生效的协议集合（nil 表示不限制）。
func (r *resolver) AllowedProtocols() map[runtime.Protocol]bool { return nil }

// Match 判断类型是否为 MongoDB 注入 wrapper。
func (r *resolver) Match(t reflect.Type) (runtime.WrapperDescriptor, bool) { return r.match(t) }

// Resolve 根据 wrapper 描述符构造实际注入值。
func (r *resolver) Resolve(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	return r.resolve(ctx, desc)
}

func matchWrapper(base string) func(reflect.Type) (runtime.WrapperDescriptor, bool) {
	return func(t reflect.Type) (runtime.WrapperDescriptor, bool) {
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct || t.PkgPath() != packagePath() || t.NumField() == 0 {
			return runtime.WrapperDescriptor{}, false
		}
		name := t.Name()
		if idx := strings.Index(name, "["); idx >= 0 {
			name = name[:idx]
		}
		if name != base {
			return runtime.WrapperDescriptor{}, false
		}
		return runtime.WrapperDescriptor{Kind: base, WrapperType: t, InnerType: t.Field(0).Type}, true
	}
}

func resolveInjectDatabase(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if ctx == nil || ctx.Container == nil {
		return nil, fmt.Errorf("mongodb binding requires request container")
	}
	value, err := ctx.Container.Get(reflect.TypeFor[*mongo.Database]())
	if err != nil {
		return nil, err
	}
	return wrap(desc.WrapperType, reflect.ValueOf(value.(*mongo.Database)))
}

func resolveInjectClient(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if ctx == nil || ctx.Container == nil {
		return nil, fmt.Errorf("mongodb binding requires request container")
	}
	value, err := ctx.Container.Get(reflect.TypeFor[*mongo.Client]())
	if err != nil {
		return nil, err
	}
	return wrap(desc.WrapperType, reflect.ValueOf(value.(*mongo.Client)))
}

func resolveDatabaseRef(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if ctx == nil || ctx.Container == nil {
		return nil, fmt.Errorf("mongodb binding requires request container")
	}
	name := databaseNameFromWrapper(desc.WrapperType)
	if value, err := ctx.Container.Get(DatabaseToken(name)); err == nil {
		if db, ok := value.(*mongo.Database); ok {
			return wrap(desc.WrapperType, reflect.ValueOf(db))
		}
	}
	value, err := ctx.Container.Get(reflect.TypeFor[*mongo.Database]())
	if err != nil {
		return nil, err
	}
	return wrap(desc.WrapperType, reflect.ValueOf(value.(*mongo.Database)))
}

func packagePath() string {
	return reflect.TypeOf(InjectDatabase{}).PkgPath()
}

func databaseNameFromWrapper(wrapperType reflect.Type) string {
	if wrapperType.Kind() == reflect.Ptr {
		wrapperType = wrapperType.Elem()
	}
	if wrapperType.Kind() != reflect.Struct || wrapperType.NumField() < 2 {
		return DefaultName
	}
	markerType := wrapperType.Field(wrapperType.NumField() - 1).Type
	if markerType.Kind() == reflect.Ptr {
		markerType = markerType.Elem()
	}
	marker := reflect.New(markerType)
	if marker.CanInterface() {
		if namer, ok := marker.Interface().(DatabaseNamer); ok {
			if name := namer.MongoDatabaseName(); name != "" {
				return name
			}
		}
	}
	return strings.ToLower(markerType.Name())
}

func wrap(wrapperType reflect.Type, value reflect.Value) (any, error) {
	if wrapperType.Kind() == reflect.Ptr {
		wrapperType = wrapperType.Elem()
	}
	target := reflect.New(wrapperType).Elem()
	target.Field(0).Set(value)
	return target.Interface(), nil
}
