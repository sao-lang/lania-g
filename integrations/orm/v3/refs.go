// refs.go 定义 orm integration 的命名引用 wrapper 与 binding 注册辅助。
package orm

import (
	"fmt"
	"reflect"
	"unsafe"

	coredi "github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"

	"gorm.io/gorm"
)

// DataSourceNamer 约定命名 datasource 引用的名称来源。
type DataSourceNamer interface {
	ORMDataSourceName() string
}

// DefaultDataSource 用于标记默认 datasource。
type DefaultDataSource struct{}

// ORMDataSourceName 返回默认 datasource 名称。
func (DefaultDataSource) ORMDataSourceName() string { return "default" }

// InjectDataSource 保留 v2 的默认 datasource 注入调用形态，
// 但当前由 v3 binding 解析，而不是通过装饰器实现。
type InjectDataSource struct {
	*gorm.DB
}

// DataSourceRef 通过标记类型 `N` 解析一个命名 datasource。
// 如果 `N` 实现了 `DataSourceNamer`，则使用其名称；否则使用类型名。
type DataSourceRef[N any] struct {
	*gorm.DB
	_ *N
}

// InjectRepository 保留 v2 的默认 repository 注入调用形态，
// 但当前由 v3 binding 解析，而不是通过装饰器实现。
type InjectRepository[T any] struct {
	*Repository[T]
	_ *T
}

// RepositoryRef 通过标记类型 `N` 解析绑定到某个命名 datasource 的 repository。
type RepositoryRef[T any, N any] struct {
	*Repository[T]
	_ *T
	_ *N
}

// DataSourceToken 返回某个命名 datasource 对应的 DI token。
func DataSourceToken(name string) string {
	if name == "" {
		name = "default"
	}
	return "orm:datasource:" + name
}

// ConfigToken 返回某个命名 datasource 配置对应的 DI token。
func ConfigToken(name string) string {
	if name == "" {
		name = "default"
	}
	return "orm:config:" + name
}

// RepositoryToken 返回某个 datasource 下实体 repository 对应的 DI token。
func RepositoryToken(name string, entityType reflect.Type) string {
	if name == "" {
		name = "default"
	}
	if entityType.Kind() == reflect.Ptr {
		entityType = entityType.Elem()
	}
	return "orm:repository:" + name + ":" + entityType.PkgPath() + "." + entityType.Name()
}

// RegisterBindings 安装 ORM 专用的参数 binding，
// 不再依赖装饰器语义，泛型包装类型会在运行时通过反射构造。
func RegisterBindings(reg *registry.Registry) {
	if reg == nil {
		RegisterBindingsCompat()
		return
	}
	registerBindings(reg)
}

// RegisterBindingsCompat 显式保留“写入全局 registry”的兼容 binding 注册入口。
func RegisterBindingsCompat() {
	registerBindings(registry.GlobalWithUsage("integrations/orm.RegisterBindingsCompat"))
}

func registerBindings(reg *registry.Registry) {
	reg.RegisterBindings(runtime.NewBindingResolvers(
		registration("InjectDataSource", coreintegration.MatchNamedWrapper(packagePath(), "InjectDataSource"), resolveInjectDataSource),
		registration("DataSourceRef", coreintegration.MatchNamedWrapper(packagePath(), "DataSourceRef"), resolveDataSourceRef),
		registration("InjectRepository", coreintegration.MatchNamedWrapper(packagePath(), "InjectRepository"), resolveInjectRepository),
		registration("RepositoryRef", coreintegration.MatchNamedWrapper(packagePath(), "RepositoryRef"), resolveRepositoryRef),
	)...)
}

// --- 解析器 ---
func resolveInjectDataSource(ctx *runtime.HandlerContext, _ runtime.WrapperDescriptor) (any, error) {
	db, err := resolveNamedDataSource(ctx, "default")
	if err != nil {
		return nil, err
	}
	return &InjectDataSource{DB: db}, nil
}

func resolveDataSourceRef(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	name := dataSourceNameFromWrapper(desc.WrapperType)
	db, err := resolveNamedDataSource(ctx, name)
	if err != nil {
		return nil, err
	}
	return wrapAnonymousField(desc.WrapperType, reflect.ValueOf(db))
}

func resolveInjectRepository(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	return resolveRepositoryForWrapper(ctx, desc.WrapperType, "default")
}

func resolveRepositoryRef(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	name := dataSourceNameFromWrapper(desc.WrapperType)
	return resolveRepositoryForWrapper(ctx, desc.WrapperType, name)
}

func resolveNamedDataSource(ctx *runtime.HandlerContext, name string) (*gorm.DB, error) {
	if ctx == nil || ctx.Container == nil {
		return nil, fmt.Errorf("orm datasource binding requires request container")
	}
	if name != "" {
		if dbAny, err := ctx.Container.Get(DataSourceToken(name)); err == nil {
			if db, ok := dbAny.(*gorm.DB); ok {
				return db, nil
			}
		}
	}
	dbAny, err := coredi.GetByType[*gorm.DB](ctx.Container)
	if err != nil {
		return nil, err
	}
	return dbAny, nil
}

func resolveRepositoryForWrapper(ctx *runtime.HandlerContext, wrapperType reflect.Type, name string) (any, error) {
	if ctx == nil || ctx.Container == nil {
		return nil, fmt.Errorf("orm repository binding requires request container")
	}
	db, err := resolveNamedDataSource(ctx, name)
	if err != nil {
		return nil, err
	}
	if wrapperType.Kind() == reflect.Ptr {
		wrapperType = wrapperType.Elem()
	}
	if wrapperType.Kind() != reflect.Struct || wrapperType.NumField() == 0 {
		return nil, fmt.Errorf("invalid orm wrapper type: %s", wrapperType.String())
	}
	repoPtrType := wrapperType.Field(0).Type
	if repoPtrType.Kind() != reflect.Ptr || repoPtrType.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("invalid repository pointer type: %s", repoPtrType.String())
	}
	repoPtr := reflect.New(repoPtrType.Elem())
	dbField := repoPtr.Elem().FieldByName("db")
	if !dbField.IsValid() {
		return nil, fmt.Errorf("repository %s missing db field", repoPtrType.String())
	}
	forceSet(dbField, reflect.ValueOf(db))
	return wrapAnonymousField(wrapperType, repoPtr)
}

func dataSourceNameFromWrapper(wrapperType reflect.Type) string {
	return coreintegration.ResolveMarkerName(wrapperType, "default", func(marker any) (string, bool) {
		namer, ok := marker.(DataSourceNamer)
		if !ok {
			return "", false
		}
		return namer.ORMDataSourceName(), true
	})
}

func wrapAnonymousField(wrapperType reflect.Type, value reflect.Value) (any, error) {
	if wrapperType.Kind() == reflect.Ptr {
		wrapperType = wrapperType.Elem()
	}
	target := reflect.New(wrapperType).Elem()
	if target.NumField() == 0 {
		return nil, fmt.Errorf("wrapper %s has no fields", wrapperType.String())
	}
	field := target.Field(0)
	forceSet(field, value)
	return target.Interface(), nil
}

func forceSet(field reflect.Value, value reflect.Value) {
	if field.CanSet() {
		field.Set(value)
		return
	}
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(value)
}

func registration(name string, match func(reflect.Type) (runtime.WrapperDescriptor, bool), resolve func(*runtime.HandlerContext, runtime.WrapperDescriptor) (any, error)) runtime.BindingRegistration {
	return runtime.BindingRegistration{
		Name:    name,
		Match:   match,
		Resolve: resolve,
	}
}

func packagePath() string {
	return reflect.TypeFor[DataSourceNamer]().PkgPath()
}
