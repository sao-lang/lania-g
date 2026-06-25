// feature.go 实现 orm 集成的可选特性开关与能力检测逻辑。
package orm

import (
	"fmt"
	"reflect"

	"gorm.io/gorm"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

// Feature 描述某个 datasource 下需要注册的实体与迁移信息。
type Feature struct {
	DataSourceName string
	Entities       []any
	AutoMigrate    []any
}

// FeatureOptions 描述 ORM feature module 的导入与功能配置。
type FeatureOptions struct {
	Imports  []module.Module
	Features []Feature
}

type featureModule struct {
	*module.BaseModule
	options FeatureOptions
}

// ForFeature 根据一组 feature 声明创建 ORM feature module。
func ForFeature(features ...Feature) (module.Module, error) {
	return ForFeatureWithOptions(FeatureOptions{Features: features})
}

// ForFeatureWithOptions 根据完整选项创建 ORM feature module。
func ForFeatureWithOptions(opts FeatureOptions) (module.Module, error) {
	base := module.NewBaseModule(&module.ModuleMetadata{
		Imports:   append([]module.Module{}, opts.Imports...),
		Providers: []*di.Provider{},
		Exports:   []any{},
	})
	return &featureModule{BaseModule: base, options: opts}, nil
}

// Init 初始化 feature module，并注册 feature 对应的数据源和 repository。
func (m *featureModule) Init() error {
	if len(m.options.Features) > 0 {
		if err := m.prepare(); err != nil {
			return err
		}
	}
	return m.BaseModule.Init()
}

func (m *featureModule) prepare() error {
	meta := m.Metadata()
	for _, imported := range meta.Imports {
		if imported == nil {
			continue
		}
		if err := imported.Init(); err != nil {
			return err
		}
	}
	factoryAny, err := m.resolveFromImports(reflect.TypeFor[Factory]())
	if err != nil {
		return err
	}
	factory := factoryAny.(Factory)
	for _, feature := range m.options.Features {
		name := feature.DataSourceName
		if name == "" {
			name = "default"
		}
		db, err := m.resolveOrCreateDataSource(factory, name)
		if err != nil {
			return err
		}
		if len(feature.AutoMigrate) > 0 {
			if err := db.AutoMigrate(feature.AutoMigrate...); err != nil {
				return err
			}
		}
		if err := m.registerDataSource(name, db); err != nil {
			return err
		}
		for _, entity := range feature.Entities {
			if entity == nil {
				continue
			}
			if meta := GetEntity(entity); meta == nil {
				registerEntityFromType(entity)
			}
			if err := m.registerRepository(name, entity); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *featureModule) resolveFromImports(token any) (any, error) {
	for _, imported := range m.Metadata().Imports {
		if imported == nil {
			continue
		}
		if value, err := imported.Container().Get(token); err == nil {
			return value, nil
		}
	}
	return nil, fmt.Errorf("orm feature import does not export token: %v", token)
}

func (m *featureModule) resolveOrCreateDataSource(factory Factory, name string) (*gorm.DB, error) {
	for _, imported := range m.Metadata().Imports {
		if imported == nil {
			continue
		}
		if value, err := imported.Container().Get(DataSourceToken(name)); err == nil {
			if db, ok := value.(*gorm.DB); ok {
				return db, nil
			}
		}
	}
	return factory.GetOrCreate(name, Config{Name: name})
}

func (m *featureModule) registerDataSource(name string, db *gorm.DB) error {
	token := DataSourceToken(name)
	p, err := di.ProviderFromInstanceWithToken(token, db, di.Singleton)
	if err != nil {
		return err
	}
	m.Metadata().Providers = append(m.Metadata().Providers, p)
	m.Metadata().Exports = append(m.Metadata().Exports, token)
	return nil
}

func (m *featureModule) registerRepository(name string, entity any) error {
	t := reflect.TypeOf(entity)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	token := RepositoryToken(name, t)
	dbAny, err := m.resolveFromImports(DataSourceToken(name))
	if err != nil {
		dbAny, err = m.resolveFromImports(reflect.TypeFor[*gorm.DB]())
		if err != nil {
			return err
		}
	}
	// Repository[T] 无法在运行时通过反射实例化为可强类型注入，因此这里只导出 token，
	// handler/constructor 侧通过 InjectRepository[T] / RepositoryRef[T, N] 绑定解析。
	p, err := di.ProviderFromInstanceWithToken(token, dbAny, di.Singleton)
	if err != nil {
		return err
	}
	m.Metadata().Providers = append(m.Metadata().Providers, p)
	m.Metadata().Exports = append(m.Metadata().Exports, token)
	return nil
}

func registerEntityFromType(entity any) {
	t := reflect.TypeOf(entity)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Name() == "" {
		return
	}
	Entity(t.Name(), entity)
}
