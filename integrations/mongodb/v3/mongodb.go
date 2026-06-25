// mongodb.go 实现 mongodb 集成的底层客户端封装与连接能力。
package mongodb

import (
	"sync"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// DefaultName 是默认 MongoDB 实例名。
const DefaultName = "default"

// Config 描述 MongoDB integration 的连接配置。
type Config struct {
	Name     string
	URI      string
	Database string
	Client   *mongo.Client
	DB       *mongo.Database
	Options  *options.ClientOptions
}

// Factory 定义 MongoDB Database 工厂接口。
type Factory interface {
	Default() *mongo.Database
	New(cfg Config) (*mongo.Database, error)
	GetOrCreate(name string, cfg Config) (*mongo.Database, error)
}

// Module 是 MongoDB integration 的模块封装。
type Module struct {
	*module.BaseModule
	client       *mongo.Client
	db           *mongo.Database
	factory      *factory
	config       Config
	reg          *registry.Registry
	compatSource string
}

type factory struct {
	mu        sync.RWMutex
	defaultDB *mongo.Database
	dbs       map[string]*mongo.Database
	clients   map[string]*mongo.Client
}

// DatabaseNamer 允许 marker 类型自定义 `DatabaseRef` 对应的数据库名称。
type DatabaseNamer interface {
	MongoDatabaseName() string
}

// InjectDatabase 用于在 handler 参数中注入默认 `*mongo.Database`。
type InjectDatabase struct{ *mongo.Database }

// InjectClient 用于在 handler 参数中注入默认 `*mongo.Client`。
type InjectClient struct{ *mongo.Client }

// DatabaseRef 用于在 handler 参数中注入“按名字区分”的数据库实例。
type DatabaseRef[N any] struct {
	*mongo.Database
	_ *N
}

// DefaultConfig 返回一份默认配置。
func DefaultConfig() Config {
	return Config{Name: DefaultName, URI: "mongodb://127.0.0.1:27017"}
}

func normalizeConfig(cfg Config) Config {
	def := DefaultConfig()
	if cfg.Name == "" {
		cfg.Name = def.Name
	}
	if cfg.URI == "" && cfg.Client == nil && cfg.DB == nil {
		cfg.URI = def.URI
	}
	return cfg
}

// DatabaseToken 返回按 name 区分的数据库 token。
func DatabaseToken(name string) string {
	if name == "" {
		name = DefaultName
	}
	return "mongodb:database:" + name
}
