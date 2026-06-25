// module.go 负责把 orm integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package orm

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"

	"gorm.io/gorm"
)

// Config 描述单个 ORM datasource 的初始化配置。
type Config struct {
	Name        string
	DB          *gorm.DB
	Dialector   gorm.Dialector
	GormConfig  *gorm.Config
	AutoMigrate []interface{}
	Plugins     []gorm.Plugin
	Managed     bool
}

// MultiConfig 描述多数据源场景下的 ORM 初始化配置。
type MultiConfig struct {
	DefaultName string
	DataSources []Config
}

// Factory 约定 ORM 数据源工厂需要提供的能力。
type Factory interface {
	Default() *gorm.DB
	New(cfg Config) (*gorm.DB, error)
	GetOrCreate(name string, cfg Config) (*gorm.DB, error)
	CloseAll() error
}

// Module 是 ORM integration 对应的模块封装。
type Module struct {
	*module.BaseModule
	db           *gorm.DB
	factory      *factory
	config       Config
	reg          *registry.Registry
	compatSource string
}

type factory struct {
	mu          sync.RWMutex
	defaultName string
	defaultDB   *gorm.DB
	dbs         map[string]*gorm.DB
	managed     map[string]bool
}

// ForRoot 使用单个 datasource 配置创建 ORM 模块。
func ForRoot(cfg Config) (module.Module, error) {
	return ForRootMulti(MultiConfig{DefaultName: cfg.Name, DataSources: []Config{cfg}})
}

// ForRootCompat 显式保留 standalone Init 写入全局 registry 的兼容语义。
func ForRootCompat(cfg Config) (module.Module, error) {
	return ForRootMultiCompat(MultiConfig{DefaultName: cfg.Name, DataSources: []Config{cfg}})
}

// ForRoots 使用多份 datasource 配置创建 ORM 模块。
func ForRoots(configs ...Config) (module.Module, error) {
	return ForRootMulti(MultiConfig{DataSources: configs})
}

// ForRootsCompat 显式保留 standalone Init 写入全局 registry 的兼容语义。
func ForRootsCompat(configs ...Config) (module.Module, error) {
	return ForRootMultiCompat(MultiConfig{DataSources: configs})
}

// ForRootMulti 使用多数据源配置创建 ORM 模块。
func ForRootMulti(cfg MultiConfig) (module.Module, error) {
	configs, defaultName, err := normalizeMultiConfig(cfg)
	if err != nil {
		return nil, err
	}
	f := newFactory(defaultName)
	providers := make([]*di.Provider, 0, len(configs)*3+4)
	exports := make([]interface{}, 0, len(configs)*3+4)
	configMap := make(map[string]Config, len(configs))

	for _, item := range configs {
		db, err := f.GetOrCreate(item.Name, item)
		if err != nil {
			return nil, err
		}
		configMap[item.Name] = item

		namedDBToken := DataSourceToken(item.Name)
		namedConfigToken := ConfigToken(item.Name)

		pNamedDB, err := di.ProviderFromInstanceWithToken(namedDBToken, db, di.Singleton)
		if err != nil {
			return nil, err
		}
		pNamedConfig, err := di.ProviderFromInstanceWithToken(namedConfigToken, item, di.Singleton)
		if err != nil {
			return nil, err
		}
		pNamedConfigPtr, err := di.ProviderFromInstanceWithToken(namedConfigToken+"@ptr", &item, di.Singleton)
		if err != nil {
			return nil, err
		}
		providers = append(providers, pNamedDB, pNamedConfig, pNamedConfigPtr)
		exports = append(exports, namedDBToken, namedConfigToken, namedConfigToken+"@ptr")
	}

	defaultConfig := configMap[defaultName]
	defaultDB := f.Default()
	dbToken := reflect.TypeFor[*gorm.DB]()
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()
	txManagerToken := TransactionManagerToken()

	pDB, err := di.ProviderFromInstanceWithToken(dbToken, defaultDB, di.Singleton)
	if err != nil {
		return nil, err
	}
	pConfigPtr, err := di.ProviderFromInstanceWithToken(configPtrToken, &defaultConfig, di.Singleton)
	if err != nil {
		return nil, err
	}
	pConfigValue, err := di.ProviderFromInstanceWithToken(configValueToken, defaultConfig, di.Singleton)
	if err != nil {
		return nil, err
	}
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(f), di.Singleton)
	if err != nil {
		return nil, err
	}
	pTxManager, err := di.ProviderFromInstanceWithToken(txManagerToken, NewTransactionManager(defaultDB), di.Singleton)
	if err != nil {
		return nil, err
	}

	providers = append([]*di.Provider{pDB, pConfigPtr, pConfigValue, pFactory, pTxManager}, providers...)
	exports = append([]interface{}{dbToken, configPtrToken, configValueToken, factoryToken, txManagerToken}, exports...)

	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: providers,
		Exports:   exports,
	})
	return &Module{BaseModule: base, db: defaultDB, factory: f, config: defaultConfig}, nil
}

// ForRootMultiCompat 显式保留 standalone Init 写入全局 registry 的兼容语义。
func ForRootMultiCompat(cfg MultiConfig) (module.Module, error) {
	mod, err := ForRootMulti(cfg)
	if err != nil {
		return nil, err
	}
	if typed, ok := mod.(*Module); ok {
		typed.compatSource = "integrations/orm.ForRootMultiCompat"
	}
	return mod, nil
}

// Init 初始化 ORM 模块，并把 ORM 相关 binding 注册到 registry。
func (m *Module) Init() error {
	if err := m.BaseModule.Init(); err != nil {
		return err
	}
	txManagerToken := TransactionManagerToken()
	reg := m.reg
	if reg == nil {
		if m.compatSource == "" {
			return fmt.Errorf("orm.Module.Init requires an explicit registry; call SetRegistry(...) before Init or use orm.ForRootCompat(...) / orm.ForRootsCompat(...) / orm.ForRootMultiCompat(...) for standalone/global compatibility")
		}
		reg = registry.GlobalWithUsage(m.compatSource)
	}
	coreintegration.RegisterContainerBindings(reg,
		coreintegration.NewBindingEntry("ORMDataSource", reflect.TypeFor[*gorm.DB]()),
		coreintegration.NewBindingEntry("ORMFactory", reflect.TypeFor[Factory]()),
		coreintegration.NewBindingEntry("ORMConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("ORMConfigPtr", reflect.TypeFor[*Config]()),
		coreintegration.NewBindingEntry("ORMTransactionManager", txManagerToken),
	)
	RegisterBindings(reg)
	return nil
}

// DB 返回默认 datasource 对应的 `*gorm.DB`。
func (m *Module) DB() *gorm.DB { return m.db }

// DataSource 按名称返回某个 datasource。
func (m *Module) DataSource(name string) (*gorm.DB, bool) {
	if m.factory == nil {
		return nil, false
	}
	return m.factory.Lookup(name)
}

// Factory 返回 ORM 数据源工厂。
func (m *Module) Factory() Factory { return m.factory }

// Config 返回默认 datasource 的配置快照。
func (m *Module) Config() Config { return cloneConfig(m.config) }

// SetRegistry 注入 registry，供 Init 阶段注册 binding。
func (m *Module) SetRegistry(reg *registry.Registry) { m.reg = reg }

func normalizeConfig(cfg Config) Config {
	if cfg.Name == "" {
		cfg.Name = "default"
	}
	return cfg
}

func cloneConfig(cfg Config) Config {
	out := cfg
	out.AutoMigrate = append([]interface{}{}, cfg.AutoMigrate...)
	return out
}

func normalizeMultiConfig(cfg MultiConfig) ([]Config, string, error) {
	if len(cfg.DataSources) == 0 {
		return nil, "", fmt.Errorf("orm requires at least one datasource config")
	}
	items := make([]Config, 0, len(cfg.DataSources))
	seen := make(map[string]bool, len(cfg.DataSources))
	defaultName := cfg.DefaultName
	for idx, item := range cfg.DataSources {
		item = normalizeConfig(item)
		if idx == 0 && defaultName == "" {
			defaultName = item.Name
		}
		if seen[item.Name] {
			return nil, "", fmt.Errorf("orm datasource name duplicated: %s", item.Name)
		}
		seen[item.Name] = true
		items = append(items, item)
	}
	if defaultName == "" {
		defaultName = "default"
	}
	if !seen[defaultName] {
		return nil, "", fmt.Errorf("orm default datasource %s is not configured", defaultName)
	}
	return items, defaultName, nil
}

func newFactory(defaultName string) *factory {
	if defaultName == "" {
		defaultName = "default"
	}
	return &factory{
		defaultName: defaultName,
		dbs:         make(map[string]*gorm.DB),
		managed:     make(map[string]bool),
	}
}

// Default 返回默认 datasource。
func (f *factory) Default() *gorm.DB { return f.defaultDB }

// Lookup 按名称查找已创建的数据源。
func (f *factory) Lookup(name string) (*gorm.DB, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	db, ok := f.dbs[name]
	return db, ok
}

// New 根据配置创建一个新的 datasource。
func (f *factory) New(cfg Config) (*gorm.DB, error) {
	cfg = normalizeConfig(cfg)
	var db *gorm.DB
	switch {
	case cfg.DB != nil:
		db = cfg.DB
	case cfg.Dialector != nil:
		var err error
		db, err = gorm.Open(cfg.Dialector, cfg.GormConfig)
		if err != nil {
			return nil, err
		}
		cfg.Managed = true
	default:
		return nil, fmt.Errorf("orm requires either DB or Dialector")
	}
	if len(cfg.AutoMigrate) > 0 {
		if err := db.AutoMigrate(cfg.AutoMigrate...); err != nil {
			return nil, err
		}
	}
	for _, plugin := range cfg.Plugins {
		if plugin == nil {
			continue
		}
		if err := db.Use(plugin); err != nil {
			return nil, err
		}
	}
	return db, nil
}

// GetOrCreate 按名称获取 datasource；不存在时按配置创建。
func (f *factory) GetOrCreate(name string, cfg Config) (*gorm.DB, error) {
	if name == "" {
		name = cfg.Name
	}
	if name == "" {
		name = f.defaultName
	}
	f.mu.RLock()
	if db, ok := f.dbs[name]; ok {
		f.mu.RUnlock()
		return db, nil
	}
	f.mu.RUnlock()

	f.mu.Lock()
	defer f.mu.Unlock()
	if db, ok := f.dbs[name]; ok {
		return db, nil
	}
	cfg.Name = name
	db, err := f.New(cfg)
	if err != nil {
		return nil, err
	}
	f.dbs[name] = db
	f.managed[name] = cfg.Managed
	if name == f.defaultName || f.defaultDB == nil {
		f.defaultDB = db
	}
	return db, nil
}

// CloseAll 关闭工厂管理的所有受管 datasource。
func (f *factory) CloseAll() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var lastErr error
	for name := range f.dbs {
		if f.managed[name] {
			if sqlDB, err := f.dbs[name].DB(); err == nil && sqlDB != nil {
				if err := sqlDB.Close(); err != nil {
					lastErr = err
				}
			}
		}
		delete(f.dbs, name)
		delete(f.managed, name)
	}
	f.defaultDB = nil
	return lastErr
}

// OnModuleDestroy 在模块销毁时关闭所有受管 datasource。
func (f *factory) OnModuleDestroy() error { return f.CloseAll() }
