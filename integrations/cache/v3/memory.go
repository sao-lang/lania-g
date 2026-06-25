// memory.go 提供 cache 集成的内存后端实现。
package cache

import (
	"encoding/json"
	"fmt"
	"path"
	"sync"
	"time"
)

// MemoryCache 是基于进程内 map 的内存缓存实现。
type MemoryCache struct {
	mu    sync.RWMutex
	items map[string]*memoryItem
	stop  chan struct{}
}

type memoryItem struct {
	value      interface{}
	expiration int64
}

// NewMemoryCache 创建一个带定时清理能力的内存缓存实例。
func NewMemoryCache() *MemoryCache {
	cache := &MemoryCache{
		items: make(map[string]*memoryItem),
		stop:  make(chan struct{}),
	}
	go cache.cleanup()
	return cache
}

func (m *MemoryCache) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.mu.Lock()
			now := time.Now().UnixNano()
			for key, item := range m.items {
				if item.expiration > 0 && now > item.expiration {
					delete(m.items, key)
				}
			}
			m.mu.Unlock()
		case <-m.stop:
			return
		}
	}
}

// Get 读取一个缓存值；当 key 不存在或已过期时返回 nil。
func (m *MemoryCache) Get(key string) (interface{}, error) {
	m.mu.RLock()
	item, ok := m.items[key]
	if !ok {
		m.mu.RUnlock()
		return nil, nil
	}
	if item.expiration > 0 && time.Now().UnixNano() > item.expiration {
		m.mu.RUnlock()
		m.mu.Lock()
		delete(m.items, key)
		m.mu.Unlock()
		return nil, nil
	}
	value := item.value
	m.mu.RUnlock()
	return value, nil
}

// GetString 以 string 类型读取缓存值。
func (m *MemoryCache) GetString(key string) (string, error)   { return castGet[string](m, key) }
// GetInt 以 int 类型读取缓存值。
func (m *MemoryCache) GetInt(key string) (int, error)         { return castGet[int](m, key) }
// GetInt64 以 int64 类型读取缓存值。
func (m *MemoryCache) GetInt64(key string) (int64, error)     { return castGet[int64](m, key) }
// GetFloat64 以 float64 类型读取缓存值。
func (m *MemoryCache) GetFloat64(key string) (float64, error) { return castGet[float64](m, key) }
// GetBool 以 bool 类型读取缓存值。
func (m *MemoryCache) GetBool(key string) (bool, error)       { return castGet[bool](m, key) }

// GetJSON 按 JSON 反序列化缓存中的 []byte 到 dest。
func (m *MemoryCache) GetJSON(key string, dest interface{}) error {
	value, err := m.Get(key)
	if err != nil {
		return err
	}
	if value == nil {
		return nil
	}
	data, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("value is not []byte")
	}
	return json.Unmarshal(data, dest)
}

// Set 写入一个缓存值（永不过期）。
func (m *MemoryCache) Set(key string, value interface{}) error {
	m.mu.Lock()
	m.items[key] = &memoryItem{value: value}
	m.mu.Unlock()
	return nil
}

// SetEx 写入一个缓存值，并设置过期时间。
func (m *MemoryCache) SetEx(key string, value interface{}, expiration time.Duration) error {
	m.mu.Lock()
	m.items[key] = &memoryItem{
		value:      value,
		expiration: time.Now().Add(expiration).UnixNano(),
	}
	m.mu.Unlock()
	return nil
}

// SetJSON 将 value 序列化为 JSON 后写入缓存（永不过期）。
func (m *MemoryCache) SetJSON(key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return m.Set(key, data)
}

// SetJSONEx 将 value 序列化为 JSON 后写入缓存，并设置过期时间。
func (m *MemoryCache) SetJSONEx(key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return m.SetEx(key, data, expiration)
}

// Del 删除一个缓存键。
func (m *MemoryCache) Del(key string) error {
	m.mu.Lock()
	delete(m.items, key)
	m.mu.Unlock()
	return nil
}

// DelKeys 批量删除多个缓存键。
func (m *MemoryCache) DelKeys(keys ...string) error {
	m.mu.Lock()
	for _, key := range keys {
		delete(m.items, key)
	}
	m.mu.Unlock()
	return nil
}

// Exists 判断某个缓存键是否存在且未过期。
func (m *MemoryCache) Exists(key string) (bool, error) {
	m.mu.RLock()
	item, ok := m.items[key]
	if !ok {
		m.mu.RUnlock()
		return false, nil
	}
	if item.expiration > 0 && time.Now().UnixNano() > item.expiration {
		m.mu.RUnlock()
		return false, nil
	}
	m.mu.RUnlock()
	return true, nil
}

// Expire 设置某个键的过期时间；若 key 不存在则忽略。
func (m *MemoryCache) Expire(key string, expiration time.Duration) error {
	m.mu.Lock()
	if item, ok := m.items[key]; ok {
		item.expiration = time.Now().Add(expiration).UnixNano()
	}
	m.mu.Unlock()
	return nil
}

// TTL 返回 key 的剩余生存时间；当 key 不存在或未设置过期时返回 -1。
func (m *MemoryCache) TTL(key string) (time.Duration, error) {
	m.mu.RLock()
	item, ok := m.items[key]
	if !ok || item.expiration == 0 {
		m.mu.RUnlock()
		return -1, nil
	}
	ttl := time.Duration(item.expiration - time.Now().UnixNano())
	m.mu.RUnlock()
	if ttl < 0 {
		return -1, nil
	}
	return ttl, nil
}

// Keys 返回匹配 pattern 的所有键（pattern 使用 path.Match 语法）。
func (m *MemoryCache) Keys(pattern string) ([]string, error) {
	m.mu.RLock()
	keys := make([]string, 0)
	for key := range m.items {
		if matchPattern(key, pattern) {
			keys = append(keys, key)
		}
	}
	m.mu.RUnlock()
	return keys, nil
}

// FlushDB 清空当前缓存中的所有键。
func (m *MemoryCache) FlushDB() error {
	m.mu.Lock()
	m.items = make(map[string]*memoryItem)
	m.mu.Unlock()
	return nil
}

// Close 停止后台清理协程。
func (m *MemoryCache) Close() error {
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
	return nil
}

func castGet[T any](c Cache, key string) (T, error) {
	value, err := c.Get(key)
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

func matchPattern(key, pattern string) bool {
	matched, err := path.Match(pattern, key)
	return err == nil && matched
}
