// module.go 负责把 mongodb integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package mongodb

import (
	"fmt"
	"reflect"

	"go.mongodb.org/mongo-driver/mongo"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// ForRoot 创建 MongoDB 集成模块，并注册数据库、客户端、配置与工厂。
func ForRoot(cfg Config) (module.Module, error) {
	cfg = normalizeConfig(cfg)
	f := &factory{dbs: map[string]*mongo.Database{}, clients: map[string]*mongo.Client{}}
	db, err := f.GetOrCreate(cfg.Name, cfg)
	if err != nil {
		return nil, err
	}
	client := cfg.Client
	if client == nil && db != nil {
		client = db.Client()
	}
	dbToken := reflect.TypeFor[*mongo.Database]()
	clientToken := reflect.TypeFor[*mongo.Client]()
	namedDBToken := DatabaseToken(cfg.Name)
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()
	pDB, err := di.ProviderFromInstanceWithToken(dbToken, db, di.Singleton)
	if err != nil {
		return nil, err
	}
	pNamedDB, err := di.ProviderFromInstanceWithToken(namedDBToken, db, di.Singleton)
	if err != nil {
		return nil, err
	}
	pClient, err := di.ProviderFromInstanceWithToken(clientToken, client, di.Singleton)
	if err != nil {
		return nil, err
	}
	pConfigPtr, err := di.ProviderFromInstanceWithToken(configPtrToken, &cfg, di.Singleton)
	if err != nil {
		return nil, err
	}
	pConfigValue, err := di.ProviderFromInstanceWithToken(configValueToken, cfg, di.Singleton)
	if err != nil {
		return nil, err
	}
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(f), di.Singleton)
	if err != nil {
		return nil, err
	}
	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pDB, pNamedDB, pClient, pConfigPtr, pConfigValue, pFactory},
		Exports:   []any{dbToken, namedDBToken, clientToken, configPtrToken, configValueToken, factoryToken},
	})
	return &Module{BaseModule: base, client: client, db: db, factory: f, config: cfg}, nil
}

// ForRootCompat 显式保留 standalone Init 写入全局 registry 的兼容语义。
func ForRootCompat(cfg Config) (module.Module, error) {
	cfg = normalizeConfig(cfg)
	f := &factory{dbs: map[string]*mongo.Database{}, clients: map[string]*mongo.Client{}}
	db, err := f.GetOrCreate(cfg.Name, cfg)
	if err != nil {
		return nil, err
	}
	client := cfg.Client
	if client == nil && db != nil {
		client = db.Client()
	}
	dbToken := reflect.TypeFor[*mongo.Database]()
	clientToken := reflect.TypeFor[*mongo.Client]()
	namedDBToken := DatabaseToken(cfg.Name)
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})
	factoryToken := reflect.TypeFor[Factory]()
	pDB, err := di.ProviderFromInstanceWithToken(dbToken, db, di.Singleton)
	if err != nil {
		return nil, err
	}
	pNamedDB, err := di.ProviderFromInstanceWithToken(namedDBToken, db, di.Singleton)
	if err != nil {
		return nil, err
	}
	pClient, err := di.ProviderFromInstanceWithToken(clientToken, client, di.Singleton)
	if err != nil {
		return nil, err
	}
	pConfigPtr, err := di.ProviderFromInstanceWithToken(configPtrToken, &cfg, di.Singleton)
	if err != nil {
		return nil, err
	}
	pConfigValue, err := di.ProviderFromInstanceWithToken(configValueToken, cfg, di.Singleton)
	if err != nil {
		return nil, err
	}
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(f), di.Singleton)
	if err != nil {
		return nil, err
	}
	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pDB, pNamedDB, pClient, pConfigPtr, pConfigValue, pFactory},
		Exports:   []any{dbToken, namedDBToken, clientToken, configPtrToken, configValueToken, factoryToken},
	})
	return &Module{compatSource: "integrations/mongodb.ForRootCompat", BaseModule: base, client: client, db: db, factory: f, config: cfg}, nil
}

// Init 初始化 MongoDB 模块，并把 Mongo 相关 binding 注册到 registry。
func (m *Module) Init() error {
	if err := m.BaseModule.Init(); err != nil {
		return err
	}
	reg := m.reg
	if reg == nil {
		if m.compatSource == "" {
			return fmt.Errorf("mongodb.Module.Init requires an explicit registry; call SetRegistry(...) before Init or use mongodb.ForRootCompat(...) for standalone/global compatibility")
		}
		reg = registry.GlobalWithUsage(m.compatSource)
	}
	coreintegration.RegisterContainerBindings(reg,
		coreintegration.NewBindingEntry("MongoDatabase", reflect.TypeFor[*mongo.Database]()),
		coreintegration.NewBindingEntry("MongoClient", reflect.TypeFor[*mongo.Client]()),
		coreintegration.NewBindingEntry("MongoConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("MongoConfigPtr", reflect.TypeFor[*Config]()),
		coreintegration.NewBindingEntry("MongoFactory", reflect.TypeFor[Factory]()),
	)
	RegisterBindings(reg)
	return nil
}

// DB 返回当前模块持有的默认数据库实例。
func (m *Module) DB() *mongo.Database { return m.db }

// Client 返回当前模块持有的 Mongo client。
func (m *Module) Client() *mongo.Client { return m.client }

// Factory 返回 Mongo 工厂，用于按名称创建更多数据库实例。
func (m *Module) Factory() Factory { return m.factory }

// Config 返回当前模块使用的 Mongo 配置。
func (m *Module) Config() Config { return m.config }

// SetRegistry 注入 registry，供 Init 阶段注册绑定声明。
func (m *Module) SetRegistry(reg *registry.Registry) { m.reg = reg }
