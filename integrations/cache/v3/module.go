// module.go 负责把 cache integration 装配成可导入模块，并统一注册导出的 provider、config 与 binding。
package cache

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	coreintegration "github.com/sao-lang/lania-g/kernel/v3/integration"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// Module 是 cache integration 对应的模块封装。
type Module struct {
	*module.BaseModule
	cache        Cache
	factory      *factory
	config       Config
	reg          *registry.Registry
	compatSource string
}

type factory struct {
	mu           sync.RWMutex
	defaultName  string
	defaultCache Cache
	caches       map[string]Cache
}

// ForRoot 创建缓存集成模块，并注册默认 cache、命名 cache、工厂与配置。
func ForRoot(cfg Config) (module.Module, error) {
	cfg = normalizeConfig(cfg)
	f := newFactory(cfg.Name)
	cache, err := f.GetOrCreate(cfg.Name, cfg)
	if err != nil {
		return nil, err
	}

	cacheToken := reflect.TypeFor[Cache]()
	namedCacheToken := CacheToken(cfg.Name)
	factoryToken := reflect.TypeFor[Factory]()
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})

	pCache, err := di.ProviderFromInstanceWithToken(cacheToken, cache, di.Singleton)
	if err != nil {
		return nil, err
	}
	pNamedCache, err := di.ProviderFromInstanceWithToken(namedCacheToken, cache, di.Singleton)
	if err != nil {
		return nil, err
	}
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(f), di.Singleton)
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

	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pCache, pNamedCache, pFactory, pConfigPtr, pConfigValue},
		Exports:   []interface{}{cacheToken, namedCacheToken, factoryToken, configPtrToken, configValueToken},
	})
	return &Module{BaseModule: base, cache: cache, factory: f, config: cfg}, nil
}

// ForRootCompat 显式保留 standalone Init 写入全局 registry 的兼容语义。
func ForRootCompat(cfg Config) (module.Module, error) {
	cfg = normalizeConfig(cfg)
	f := newFactory(cfg.Name)
	cache, err := f.GetOrCreate(cfg.Name, cfg)
	if err != nil {
		return nil, err
	}

	cacheToken := reflect.TypeFor[Cache]()
	namedCacheToken := CacheToken(cfg.Name)
	factoryToken := reflect.TypeFor[Factory]()
	configPtrToken := reflect.TypeFor[*Config]()
	configValueToken := reflect.TypeOf(Config{})

	pCache, err := di.ProviderFromInstanceWithToken(cacheToken, cache, di.Singleton)
	if err != nil {
		return nil, err
	}
	pNamedCache, err := di.ProviderFromInstanceWithToken(namedCacheToken, cache, di.Singleton)
	if err != nil {
		return nil, err
	}
	pFactory, err := di.ProviderFromInstanceWithToken(factoryToken, Factory(f), di.Singleton)
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

	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pCache, pNamedCache, pFactory, pConfigPtr, pConfigValue},
		Exports:   []interface{}{cacheToken, namedCacheToken, factoryToken, configPtrToken, configValueToken},
	})
	return &Module{compatSource: "integrations/cache.ForRootCompat", BaseModule: base, cache: cache, factory: f, config: cfg}, nil
}

// Init 初始化缓存模块，并把缓存相关 binding 注册到 registry。
func (m *Module) Init() error {
	if err := m.BaseModule.Init(); err != nil {
		return err
	}
	reg := m.reg
	if reg == nil {
		if m.compatSource == "" {
			return fmt.Errorf("cache.Module.Init requires an explicit registry; call SetRegistry(...) before Init or use cache.ForRootCompat(...) for standalone/global compatibility")
		}
		reg = registry.GlobalWithUsage(m.compatSource)
	}
	coreintegration.RegisterContainerBindings(reg,
		coreintegration.NewBindingEntry("Cache", reflect.TypeFor[Cache]()),
		coreintegration.NewBindingEntry("CacheFactory", reflect.TypeFor[Factory]()),
		coreintegration.NewBindingEntry("CacheConfig", reflect.TypeOf(Config{})),
		coreintegration.NewBindingEntry("CacheConfigPtr", reflect.TypeFor[*Config]()),
	)
	RegisterBindings(reg)
	return nil
}

// Cache 返回当前模块持有的默认缓存实例。
func (m *Module) Cache() Cache { return m.cache }

// Factory 返回缓存工厂，用于按名称或配置创建更多缓存实例。
func (m *Module) Factory() Factory { return m.factory }

// Config 返回当前模块使用的缓存配置快照。
func (m *Module) Config() Config { return cloneConfig(m.config) }

// SetRegistry 注入 registry，供 Init 阶段注册绑定声明。
func (m *Module) SetRegistry(reg *registry.Registry) { m.reg = reg }

func normalizeConfig(cfg Config) Config {
	if cfg.Type == "" {
		cfg.Type = Memory
	}
	if cfg.Name == "" {
		cfg.Name = "default"
	}
	return cfg
}

func cloneConfig(cfg Config) Config {
	out := cfg
	if cfg.Redis != nil {
		redisCfg := *cfg.Redis
		out.Redis = &redisCfg
	}
	return out
}

func newFactory(defaultName string) *factory {
	if defaultName == "" {
		defaultName = "default"
	}
	return &factory{
		defaultName: defaultName,
		caches:      make(map[string]Cache),
	}
}

// Default 返回默认缓存实例。
func (f *factory) Default() Cache { return f.defaultCache }

// New 按给定配置创建一个新的缓存实例。
func (f *factory) New(cfg Config) (Cache, error) {
	cfg = normalizeConfig(cfg)
	switch cfg.Type {
	case Redis:
		return NewRedisCache(cfg.Redis), nil
	case Memory:
		fallthrough
	default:
		return NewMemoryCache(), nil
	}
}

// GetOrCreate 按名称获取缓存实例；不存在时按配置创建并缓存下来。
func (f *factory) GetOrCreate(name string, cfg Config) (Cache, error) {
	if name == "" {
		name = cfg.Name
	}
	if name == "" {
		name = f.defaultName
	}
	f.mu.RLock()
	if cache, ok := f.caches[name]; ok {
		f.mu.RUnlock()
		return cache, nil
	}
	f.mu.RUnlock()

	f.mu.Lock()
	defer f.mu.Unlock()
	if cache, ok := f.caches[name]; ok {
		return cache, nil
	}
	cfg.Name = name
	cache, err := f.New(cfg)
	if err != nil {
		return nil, err
	}
	f.caches[name] = cache
	if name == f.defaultName || f.defaultCache == nil {
		f.defaultCache = cache
	}
	return cache, nil
}

// CloseAll 关闭工厂管理的所有缓存实例。
func (f *factory) CloseAll() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	var lastErr error
	for name, cache := range f.caches {
		if err := cache.Close(); err != nil {
			lastErr = err
		}
		delete(f.caches, name)
	}
	f.defaultCache = nil
	return lastErr
}

// OnModuleDestroy 在模块销毁时关闭全部缓存实例。
func (f *factory) OnModuleDestroy() error { return f.CloseAll() }

// Must 是泛型版缓存读取辅助函数；读取失败时直接 panic。
func Must[T any](cache Cache, key string) T {
	value, err := Get[T](cache, key)
	if err != nil {
		panic(err)
	}
	return value
}

// Get 是泛型版缓存读取辅助函数。
func Get[T any](cache Cache, key string) (T, error) {
	value, err := cache.Get(key)
	if err != nil {
		var zero T
		return zero, err
	}
	if value == nil {
		var zero T
		return zero, nil
	}
	out, ok := value.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("type assertion failed: expected %T, got %T", zero, value)
	}
	return out, nil
}

// SetValue 是泛型版缓存写入辅助函数。
func SetValue[T any](cache Cache, key string, value T) error { return cache.Set(key, value) }
