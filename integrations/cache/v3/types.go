// types.go 定义 cache 集成对外暴露的公共类型、选项与包装结构。
package cache

import "time"

// CacheType 表示缓存实现类型。
type CacheType string

const (
	// Memory 表示使用内存缓存实现。
	Memory CacheType = "memory"
	// Redis 表示使用 Redis 缓存实现。
	Redis CacheType = "redis"
)

// Cache 定义框架统一使用的缓存抽象。
type Cache interface {
	Get(key string) (interface{}, error)
	GetString(key string) (string, error)
	GetInt(key string) (int, error)
	GetInt64(key string) (int64, error)
	GetFloat64(key string) (float64, error)
	GetBool(key string) (bool, error)
	GetJSON(key string, dest interface{}) error
	Set(key string, value interface{}) error
	SetEx(key string, value interface{}, expiration time.Duration) error
	SetJSON(key string, value interface{}) error
	SetJSONEx(key string, value interface{}, expiration time.Duration) error
	Del(key string) error
	DelKeys(keys ...string) error
	Exists(key string) (bool, error)
	Expire(key string, expiration time.Duration) error
	TTL(key string) (time.Duration, error)
	Keys(pattern string) ([]string, error)
	FlushDB() error
	Close() error
}

// RedisExtended 在基础 Cache 之上补充 Redis 特有操作。
type RedisExtended interface {
	Cache
	SetNX(key string, value interface{}) (bool, error)
	Incr(key string) (int64, error)
	IncrBy(key string, increment int64) (int64, error)
	Decr(key string) (int64, error)
	DecrBy(key string, decrement int64) (int64, error)
	HGet(key, field string) (interface{}, error)
	HGetString(key, field string) (string, error)
	HGetAll(key string) (map[string]string, error)
	HSet(key, field string, value interface{}) error
	HMSet(key string, fields map[string]interface{}) error
	HDel(key string, fields ...string) error
	HExists(key, field string) (bool, error)
	LPush(key string, values ...interface{}) error
	RPush(key string, values ...interface{}) error
	LPop(key string) (string, error)
	RPop(key string) (string, error)
	LRange(key string, start, stop int) ([]string, error)
	LLen(key string) (int, error)
	SAdd(key string, members ...interface{}) error
	SRem(key string, members ...interface{}) error
	SMembers(key string) ([]string, error)
	SIsMember(key string, member interface{}) (bool, error)
	ZAdd(key string, score float64, member interface{}) error
	ZRange(key string, start, stop int) ([]string, error)
	ZRank(key string, member interface{}) (int, error)
	ZScore(key string, member interface{}) (float64, error)
	FlushAll() error
	Ping() error
}

// RedisConfig 描述 Redis 缓存的连接配置。
type RedisConfig struct {
	Name         string
	Host         string
	Port         int
	Password     string
	DB           int
	MaxIdle      int
	MaxActive    int
	IdleTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// Config 描述缓存 integration 的初始化配置。
type Config struct {
	Type  CacheType
	Name  string
	Redis *RedisConfig
}

// Factory 约定缓存工厂需要提供的能力。
type Factory interface {
	Default() Cache
	New(cfg Config) (Cache, error)
	GetOrCreate(name string, cfg Config) (Cache, error)
	CloseAll() error
}
