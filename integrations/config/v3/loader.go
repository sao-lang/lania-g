// loader.go 实现 config 集成的配置加载、合并与反序列化逻辑。
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Format 表示配置文件的序列化格式。
type Format string

const (
	// JSONFormat 表示 JSON 配置文件格式。
	JSONFormat Format = "json"
	// YAMLFormat 表示 YAML 配置文件格式。
	YAMLFormat Format = "yaml"
	// YMLFormat 表示使用 yml 扩展名的 YAML 配置文件格式。
	YMLFormat  Format = "yml"
	// TOMLFormat 表示 TOML 配置文件格式。
	TOMLFormat Format = "toml"
)

// Config 描述配置加载器的初始化选项。
type Config struct {
	Files     []string
	EnvPrefix string
	Data      map[string]any
}

// Loader 是一个支持文件、环境变量与内存 map 合并的配置加载器。
type Loader struct {
	data  map[string]any
	mu    sync.RWMutex
	paths []string
}

// Factory 定义 Loader 的工厂接口。
type Factory interface {
	Default() *Loader
	New(cfg Config) (*Loader, error)
}

// NewLoader 基于配置创建一个配置加载器。
func NewLoader(cfg Config) (*Loader, error) {
	loader := &Loader{data: make(map[string]any)}
	if len(cfg.Files) > 0 {
		if err := loader.LoadFiles(cfg.Files...); err != nil {
			return nil, err
		}
	}
	if cfg.EnvPrefix != "" {
		loader.LoadFromEnv(cfg.EnvPrefix)
	}
	if len(cfg.Data) > 0 {
		loader.LoadFromMap(cfg.Data)
	}
	return loader, nil
}

// Default 返回当前 Loader 本身，用于满足 Factory 接口。
func (c *Loader) Default() *Loader { return c }

// New 基于 cfg 创建一个新的 Loader，用于满足 Factory 接口。
func (c *Loader) New(cfg Config) (*Loader, error) { return NewLoader(cfg) }

// LoadFile 读取并合并一个配置文件。
func (c *Loader) LoadFile(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	var temp map[string]any
	switch Format(ext) {
	case JSONFormat:
		err = json.Unmarshal(data, &temp)
	case YAMLFormat, YMLFormat:
		err = yaml.Unmarshal(data, &temp)
	case TOMLFormat:
		err = toml.Unmarshal(data, &temp)
	default:
		return fmt.Errorf("unsupported config format: %s", ext)
	}
	if err != nil {
		return err
	}

	c.paths = append(c.paths, path)
	deepMerge(c.data, temp)
	return nil
}

// LoadFiles 按顺序读取并合并多个配置文件。
func (c *Loader) LoadFiles(paths ...string) error {
	for _, path := range paths {
		if err := c.LoadFile(path); err != nil {
			return err
		}
	}
	return nil
}

// LoadFromMap 把 map 数据合并到当前配置中。
func (c *Loader) LoadFromMap(data map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	deepMerge(c.data, data)
}

// LoadFromEnv 按前缀扫描环境变量并合并到当前配置中。
func (c *Loader) LoadFromEnv(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix = strings.ToUpper(prefix)
	if prefix != "" && !strings.HasSuffix(prefix, "_") {
		prefix += "_"
	}
	for _, env := range os.Environ() {
		key, value, ok := strings.Cut(env, "=")
		if !ok {
			continue
		}
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			continue
		}
		key = strings.TrimPrefix(key, prefix)
		key = strings.ToLower(strings.ReplaceAll(key, "_", "."))
		setNestedValue(c.data, key, value)
	}
}

func deepMerge(dst, src map[string]any) {
	for k, v := range src {
		if existing, ok := dst[k]; ok {
			if dstMap, ok1 := existing.(map[string]any); ok1 {
				if srcMap, ok2 := v.(map[string]any); ok2 {
					deepMerge(dstMap, srcMap)
					continue
				}
			}
		}
		dst[k] = v
	}
}

func setNestedValue(m map[string]any, key string, value any) {
	parts := strings.Split(key, ".")
	current := m
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}
		if _, ok := current[part]; !ok {
			current[part] = make(map[string]any)
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			return
		}
		current = next
	}
}

// Get 读取配置值；当 key 为空时返回完整配置树。
func (c *Loader) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if key == "" {
		return c.data, true
	}
	return getNestedValue(c.data, key)
}

func getNestedValue(m map[string]any, key string) (any, bool) {
	parts := strings.Split(key, ".")
	current := m
	for i, part := range parts {
		if i == len(parts)-1 {
			val, ok := current[part]
			return val, ok
		}
		next, ok := current[part].(map[string]any)
		if !ok {
			return nil, false
		}
		current = next
	}
	return nil, false
}

// GetString 以字符串形式读取配置值，并支持默认值。
func (c *Loader) GetString(key string, defaults ...string) string {
	value, ok := c.Get(key)
	if !ok {
		if len(defaults) > 0 {
			return defaults[0]
		}
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// GetBool 以布尔形式读取配置值，并支持默认值。
func (c *Loader) GetBool(key string, defaults ...bool) bool {
	value, ok := c.Get(key)
	if !ok {
		if len(defaults) > 0 {
			return defaults[0]
		}
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true") || v == "1"
	case int:
		return v == 1
	case int64:
		return v == 1
	case float64:
		return v == 1
	default:
		if len(defaults) > 0 {
			return defaults[0]
		}
		return false
	}
}

// Unmarshal 把指定 key 下的配置解码到 dest；空 key 表示解码整个配置树。
func (c *Loader) Unmarshal(key string, dest any) error {
	value, ok := c.Get(key)
	if !ok {
		return fmt.Errorf("config key not found: %s", key)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

// All 返回顶层配置 map 的浅拷贝。
func (c *Loader) All() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]any, len(c.data))
	maps.Copy(out, c.data)
	return out
}

// Paths 返回已加载配置文件路径列表的副本。
func (c *Loader) Paths() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.paths))
	copy(out, c.paths)
	return out
}

// ToString 按指定格式序列化当前配置。
func (c *Loader) ToString(format Format) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var buf bytes.Buffer
	var err error
	switch format {
	case JSONFormat:
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		err = enc.Encode(c.data)
	case YAMLFormat, YMLFormat:
		err = yaml.NewEncoder(&buf).Encode(c.data)
	case TOMLFormat:
		err = toml.NewEncoder(&buf).Encode(c.data)
	default:
		return "", fmt.Errorf("unsupported format: %s", format)
	}
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Save 把当前配置按指定格式保存到文件。
func (c *Loader) Save(path string, formats ...Format) error {
	var f Format
	if len(formats) > 0 {
		f = formats[0]
	} else {
		f = Format(strings.ToLower(strings.TrimPrefix(filepath.Ext(path), ".")))
	}
	content, err := c.ToString(f)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// Load 读取单个配置文件并解码到 target。
func Load(path string, target any) error {
	loader, err := NewLoader(Config{Files: []string{path}})
	if err != nil {
		return err
	}
	if reflect.TypeOf(target).Kind() != reflect.Ptr || reflect.ValueOf(target).Elem().Kind() != reflect.Struct {
		return fmt.Errorf("target must be a pointer to struct")
	}
	return loader.Unmarshal("", target)
}

// MustLoad 保留为兼容别名。
// Deprecated: use Load. Despite the name, this function returns error instead of panicking.
func MustLoad(path string, target any) error {
	return Load(path, target)
}
