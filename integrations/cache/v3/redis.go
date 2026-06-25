// redis.go 提供 cache 集成的 Redis 后端实现。
package cache

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gomodule/redigo/redis"
)

// RedisCache 是基于 redigo 封装的 Redis 缓存实现。
type RedisCache struct {
	pool *redis.Pool
}

// NewRedisCache 根据配置创建一个 Redis 缓存实例。
func NewRedisCache(config *RedisConfig) *RedisCache {
	if config == nil {
		config = &RedisConfig{
			Host:        "localhost",
			Port:        6379,
			DB:          0,
			MaxIdle:     10,
			MaxActive:   100,
			IdleTimeout: 240 * time.Second,
		}
	}
	if config.Host == "" {
		config.Host = "localhost"
	}
	if config.Port == 0 {
		config.Port = 6379
	}
	if config.MaxIdle == 0 {
		config.MaxIdle = 10
	}
	if config.MaxActive == 0 {
		config.MaxActive = 100
	}
	if config.IdleTimeout == 0 {
		config.IdleTimeout = 240 * time.Second
	}
	pool := &redis.Pool{
		MaxIdle:     config.MaxIdle,
		MaxActive:   config.MaxActive,
		IdleTimeout: config.IdleTimeout,
		Dial: func() (redis.Conn, error) {
			addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
			c, err := redis.Dial("tcp", addr,
				redis.DialReadTimeout(config.ReadTimeout),
				redis.DialWriteTimeout(config.WriteTimeout),
			)
			if err != nil {
				return nil, err
			}
			if config.Password != "" {
				if _, err := c.Do("AUTH", config.Password); err != nil {
					_ = c.Close()
					return nil, err
				}
			}
			if config.DB != 0 {
				if _, err := c.Do("SELECT", config.DB); err != nil {
					_ = c.Close()
					return nil, err
				}
			}
			return c, nil
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}
	return &RedisCache{pool: pool}
}

func (r *RedisCache) conn() redis.Conn { return r.pool.Get() }

// Get 读取一个缓存值；当 key 不存在时返回 nil。
func (r *RedisCache) Get(key string) (interface{}, error) {
	c := r.conn()
	defer c.Close()
	return c.Do("GET", key)
}

// GetString 以 string 类型读取缓存值。
func (r *RedisCache) GetString(key string) (string, error) {
	c := r.conn()
	defer c.Close()
	return redis.String(c.Do("GET", key))
}

// GetInt 以 int 类型读取缓存值。
func (r *RedisCache) GetInt(key string) (int, error) {
	c := r.conn()
	defer c.Close()
	return redis.Int(c.Do("GET", key))
}

// GetInt64 以 int64 类型读取缓存值。
func (r *RedisCache) GetInt64(key string) (int64, error) {
	c := r.conn()
	defer c.Close()
	return redis.Int64(c.Do("GET", key))
}

// GetFloat64 以 float64 类型读取缓存值。
func (r *RedisCache) GetFloat64(key string) (float64, error) {
	c := r.conn()
	defer c.Close()
	return redis.Float64(c.Do("GET", key))
}

// GetBool 以 bool 类型读取缓存值。
func (r *RedisCache) GetBool(key string) (bool, error) {
	c := r.conn()
	defer c.Close()
	return redis.Bool(c.Do("GET", key))
}

// GetJSON 按 JSON 反序列化缓存中的值到 dest。
func (r *RedisCache) GetJSON(key string, dest interface{}) error {
	c := r.conn()
	defer c.Close()
	data, err := redis.Bytes(c.Do("GET", key))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

// Set 写入一个缓存值（永不过期）。
func (r *RedisCache) Set(key string, value interface{}) error {
	c := r.conn()
	defer c.Close()
	_, err := c.Do("SET", key, value)
	return err
}

// SetEx 写入一个缓存值，并设置过期时间（秒级）。
func (r *RedisCache) SetEx(key string, value interface{}, expiration time.Duration) error {
	c := r.conn()
	defer c.Close()
	_, err := c.Do("SETEX", key, int(expiration/time.Second), value)
	return err
}

// SetJSON 将 value 序列化为 JSON 后写入缓存（永不过期）。
func (r *RedisCache) SetJSON(key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.Set(key, data)
}

// SetJSONEx 将 value 序列化为 JSON 后写入缓存，并设置过期时间。
func (r *RedisCache) SetJSONEx(key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.SetEx(key, data, expiration)
}

// Del 删除一个缓存键。
func (r *RedisCache) Del(key string) error {
	c := r.conn()
	defer c.Close()
	_, err := c.Do("DEL", key)
	return err
}

// DelKeys 批量删除多个缓存键。
func (r *RedisCache) DelKeys(keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	c := r.conn()
	defer c.Close()
	args := make([]interface{}, len(keys))
	for i, key := range keys {
		args[i] = key
	}
	_, err := c.Do("DEL", args...)
	return err
}

// Exists 判断某个缓存键是否存在。
func (r *RedisCache) Exists(key string) (bool, error) {
	c := r.conn()
	defer c.Close()
	return redis.Bool(c.Do("EXISTS", key))
}

// Expire 设置某个键的过期时间（秒级）。
func (r *RedisCache) Expire(key string, expiration time.Duration) error {
	c := r.conn()
	defer c.Close()
	_, err := c.Do("EXPIRE", key, int(expiration/time.Second))
	return err
}

// TTL 返回 key 的剩余生存时间（秒）。
func (r *RedisCache) TTL(key string) (time.Duration, error) {
	c := r.conn()
	defer c.Close()
	ttl, err := redis.Int64(c.Do("TTL", key))
	return time.Duration(ttl) * time.Second, err
}

// Keys 返回匹配 pattern 的键列表（Redis KEYS）。
func (r *RedisCache) Keys(pattern string) ([]string, error) {
	c := r.conn()
	defer c.Close()
	return redis.Strings(c.Do("KEYS", pattern))
}

// FlushDB 清空当前 DB。
func (r *RedisCache) FlushDB() error {
	c := r.conn()
	defer c.Close()
	_, err := c.Do("FLUSHDB")
	return err
}

// Close 关闭底层连接池。
func (r *RedisCache) Close() error { return r.pool.Close() }

// SetNX 当 key 不存在时设置值；返回是否设置成功。
func (r *RedisCache) SetNX(key string, value interface{}) (bool, error) {
	c := r.conn()
	defer c.Close()
	return redis.Bool(c.Do("SETNX", key, value))
}

// Incr 将 key 自增 1，并返回自增后的值。
func (r *RedisCache) Incr(key string) (int64, error) {
	c := r.conn()
	defer c.Close()
	return redis.Int64(c.Do("INCR", key))
}

// IncrBy 将 key 增加指定增量，并返回更新后的值。
func (r *RedisCache) IncrBy(key string, increment int64) (int64, error) {
	c := r.conn()
	defer c.Close()
	return redis.Int64(c.Do("INCRBY", key, increment))
}

// Decr 将 key 自减 1，并返回自减后的值。
func (r *RedisCache) Decr(key string) (int64, error) {
	c := r.conn()
	defer c.Close()
	return redis.Int64(c.Do("DECR", key))
}

// DecrBy 将 key 减少指定减量，并返回更新后的值。
func (r *RedisCache) DecrBy(key string, decrement int64) (int64, error) {
	c := r.conn()
	defer c.Close()
	return redis.Int64(c.Do("DECRBY", key, decrement))
}

// HGet 读取 hash 中某个 field 的值。
func (r *RedisCache) HGet(key, field string) (interface{}, error) {
	c := r.conn()
	defer c.Close()
	return c.Do("HGET", key, field)
}

// HGetString 以 string 类型读取 hash 中某个 field 的值。
func (r *RedisCache) HGetString(key, field string) (string, error) {
	c := r.conn()
	defer c.Close()
	return redis.String(c.Do("HGET", key, field))
}

// HGetAll 读取 hash 中所有 field/value。
func (r *RedisCache) HGetAll(key string) (map[string]string, error) {
	c := r.conn()
	defer c.Close()
	return redis.StringMap(c.Do("HGETALL", key))
}

// HSet 设置 hash 中某个 field 的值。
func (r *RedisCache) HSet(key, field string, value interface{}) error {
	c := r.conn()
	defer c.Close()
	_, err := c.Do("HSET", key, field, value)
	return err
}

// HMSet 批量设置 hash 中多个 field/value。
func (r *RedisCache) HMSet(key string, fields map[string]interface{}) error {
	c := r.conn()
	defer c.Close()
	args := []interface{}{key}
	for k, v := range fields {
		args = append(args, k, v)
	}
	_, err := c.Do("HMSET", args...)
	return err
}

// HDel 删除 hash 中一个或多个 field。
func (r *RedisCache) HDel(key string, fields ...string) error {
	c := r.conn()
	defer c.Close()
	args := make([]interface{}, 0, len(fields)+1)
	args = append(args, key)
	for _, f := range fields {
		args = append(args, f)
	}
	_, err := c.Do("HDEL", args...)
	return err
}

// HExists 判断 hash 中某个 field 是否存在。
func (r *RedisCache) HExists(key, field string) (bool, error) {
	c := r.conn()
	defer c.Close()
	return redis.Bool(c.Do("HEXISTS", key, field))
}

// LPush 将一个或多个值从左侧推入列表。
func (r *RedisCache) LPush(key string, values ...interface{}) error {
	return r.doList("LPUSH", key, values...)
}

// RPush 将一个或多个值从右侧推入列表。
func (r *RedisCache) RPush(key string, values ...interface{}) error {
	return r.doList("RPUSH", key, values...)
}

// LPop 从左侧弹出一个列表元素。
func (r *RedisCache) LPop(key string) (string, error) {
	c := r.conn()
	defer c.Close()
	return redis.String(c.Do("LPOP", key))
}

// RPop 从右侧弹出一个列表元素。
func (r *RedisCache) RPop(key string) (string, error) {
	c := r.conn()
	defer c.Close()
	return redis.String(c.Do("RPOP", key))
}

// LRange 返回列表指定区间内的元素。
func (r *RedisCache) LRange(key string, start, stop int) ([]string, error) {
	c := r.conn()
	defer c.Close()
	return redis.Strings(c.Do("LRANGE", key, start, stop))
}

// LLen 返回列表长度。
func (r *RedisCache) LLen(key string) (int, error) {
	c := r.conn()
	defer c.Close()
	return redis.Int(c.Do("LLEN", key))
}

// SAdd 向集合添加一个或多个成员。
func (r *RedisCache) SAdd(key string, members ...interface{}) error {
	return r.doList("SADD", key, members...)
}

// SRem 从集合移除一个或多个成员。
func (r *RedisCache) SRem(key string, members ...interface{}) error {
	return r.doList("SREM", key, members...)
}

// SMembers 返回集合中的所有成员。
func (r *RedisCache) SMembers(key string) ([]string, error) {
	c := r.conn()
	defer c.Close()
	return redis.Strings(c.Do("SMEMBERS", key))
}

// SIsMember 判断 member 是否为集合成员。
func (r *RedisCache) SIsMember(key string, member interface{}) (bool, error) {
	c := r.conn()
	defer c.Close()
	return redis.Bool(c.Do("SISMEMBER", key, member))
}

// ZAdd 向有序集合添加一个成员及其 score。
func (r *RedisCache) ZAdd(key string, score float64, member interface{}) error {
	c := r.conn()
	defer c.Close()
	_, err := c.Do("ZADD", key, score, member)
	return err
}

// ZRange 返回有序集合中指定区间的成员列表。
func (r *RedisCache) ZRange(key string, start, stop int) ([]string, error) {
	c := r.conn()
	defer c.Close()
	return redis.Strings(c.Do("ZRANGE", key, start, stop))
}

// ZRank 返回 member 在有序集合中的排名。
func (r *RedisCache) ZRank(key string, member interface{}) (int, error) {
	c := r.conn()
	defer c.Close()
	return redis.Int(c.Do("ZRANK", key, member))
}

// ZScore 返回 member 在有序集合中的 score。
func (r *RedisCache) ZScore(key string, member interface{}) (float64, error) {
	c := r.conn()
	defer c.Close()
	return redis.Float64(c.Do("ZSCORE", key, member))
}

// FlushAll 清空所有 DB（Redis FLUSHALL）。
func (r *RedisCache) FlushAll() error {
	c := r.conn()
	defer c.Close()
	_, err := c.Do("FLUSHALL")
	return err
}

// Ping 对 Redis 执行一次 PING，用于连通性检测。
func (r *RedisCache) Ping() error { c := r.conn(); defer c.Close(); _, err := c.Do("PING"); return err }

func (r *RedisCache) doList(cmd, key string, values ...interface{}) error {
	c := r.conn()
	defer c.Close()
	args := make([]interface{}, 0, len(values)+1)
	args = append(args, key)
	args = append(args, values...)
	_, err := c.Do(cmd, args...)
	return err
}
