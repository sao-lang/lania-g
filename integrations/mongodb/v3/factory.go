// factory.go 实现 mongodb 集成的工厂构造与实例创建逻辑。
package mongodb

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Default 返回默认数据库实例。
func (f *factory) Default() *mongo.Database { return f.defaultDB }

// New 基于配置创建一个数据库实例。
func (f *factory) New(cfg Config) (*mongo.Database, error) {
	cfg = normalizeConfig(cfg)
	if cfg.DB != nil {
		return cfg.DB, nil
	}
	client := cfg.Client
	if client == nil {
		opts := cfg.Options
		if opts == nil {
			opts = options.Client().ApplyURI(cfg.URI)
		}
		var err error
		client, err = mongo.Connect(context.Background(), opts)
		if err != nil {
			return nil, err
		}
	}
	if cfg.Database == "" {
		return nil, fmt.Errorf("mongodb database name is required")
	}
	return client.Database(cfg.Database), nil
}

// GetOrCreate 按 name 获取已缓存数据库；若不存在则按 cfg 创建并缓存。
func (f *factory) GetOrCreate(name string, cfg Config) (*mongo.Database, error) {
	if name == "" {
		name = cfg.Name
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
	db, err := f.New(cfg)
	if err != nil {
		return nil, err
	}
	f.dbs[name] = db
	if db != nil && db.Client() != nil {
		f.clients[name] = db.Client()
	}
	if f.defaultDB == nil || name == DefaultName {
		f.defaultDB = db
	}
	return db, nil
}
