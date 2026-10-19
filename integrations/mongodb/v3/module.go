// module.go 负责把 mongodb integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package mongodb

import (
	"fmt"
	"reflect"

	"go.mongodb.org/mongo-driver/mongo"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
)

// build 创建 MongoDB 集成模块，并注册数据库、客户端、配置与工厂。
func build(cfg Config) (module.Module, error) {
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
	base := module.New(module.Metadata{
		Providers: []*di.Provider{pDB, pNamedDB, pClient, pConfigPtr, pConfigValue, pFactory},
		Exports:   []any{dbToken, namedDBToken, clientToken, configPtrToken, configValueToken, factoryToken},
	})
	return &integrationModule{BaseModule: base, client: client, db: db, factory: f, config: cfg}, nil
}

// Init 初始化 MongoDB 模块，并把 Mongo 相关 binding 注册到 registry。
// Module 创建 mongodb 集成模块。
//
// 它是该集成唯一的装配入口：把模块加入根模块的 Imports 即可。
func Module(cfgs ...Config) module.Module {
	if len(cfgs) == 0 {
		return module.Failed(fmt.Errorf("mongodb.Module requires at least one Config"))
	}
	modules := make([]module.Module, 0, len(cfgs))
	for _, cfg := range cfgs {
		m, err := build(cfg)
		if err != nil {
			return module.Failed(err)
		}
		modules = append(modules, m)
	}
	if len(modules) == 1 {
		return modules[0]
	}
	return module.New(module.Metadata{Imports: modules})
}

func (m *integrationModule) Init() error {
	if err := m.BaseModule.Init(); err != nil {
		return err
	}
	reg := m.Context().Registry()
	coreintegration.RegisterContainerBindings(reg,
		coreintegration.NewBindingEntry("MongoDatabase", reflect.TypeFor[*mongo.Database]()),
		coreintegration.NewBindingEntry("MongoClient", reflect.TypeFor[*mongo.Client]()),
		coreintegration.NewBindingEntry("MongoConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("MongoConfigPtr", reflect.TypeFor[*Config]()),
		coreintegration.NewBindingEntry("MongoFactory", reflect.TypeFor[Factory]()),
	)
	registerBindings(reg)
	return nil
}

// DB 返回当前模块持有的默认数据库实例。
func (m *integrationModule) DB() *mongo.Database { return m.db }

// Client 返回当前模块持有的 Mongo client。
func (m *integrationModule) Client() *mongo.Client { return m.client }

// Factory 返回 Mongo 工厂，用于按名称创建更多数据库实例。
func (m *integrationModule) Factory() Factory { return m.factory }

// Config 返回当前模块使用的 Mongo 配置。
func (m *integrationModule) Config() Config { return m.config }

