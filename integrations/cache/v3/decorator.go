// decorator.go 实现 cache 集成的装饰器能力与缓存包装逻辑。
package cache

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// KeyBuilder 根据函数调用参数生成缓存 key。
type KeyBuilder func(args []reflect.Value) (string, error)

// Policy 描述读写缓存时采用的行为策略。
type Policy struct {
	TTL             time.Duration
	SkipReadErrors  bool
	SkipWriteErrors bool
	CacheNil        bool
}

// DecoratorOptions 描述函数缓存装饰器的配置项。
type DecoratorOptions struct {
	Key                string
	KeyBuilder         KeyBuilder
	Policy             Policy
	Invalidate         []string
	InvalidatePatterns []string
}

// DefaultKeyBuilder 创建一个基于参数摘要的默认 key 生成器。
func DefaultKeyBuilder(prefix string) KeyBuilder {
	return func(args []reflect.Value) (string, error) {
		parts := make([]string, 0, len(args)+1)
		parts = append(parts, prefix)
		for _, arg := range args {
			parts = append(parts, stringifyArg(arg))
		}
		sum := sha1.Sum([]byte(strings.Join(parts, "|")))
		return prefix + ":" + hex.EncodeToString(sum[:]), nil
	}
}

// Invalidate 删除指定的一组缓存 key。
func Invalidate(cache Cache, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return cache.DelKeys(keys...)
}

// InvalidatePattern 按模式匹配并删除缓存 key。
func InvalidatePattern(cache Cache, patterns ...string) error {
	if cache == nil || len(patterns) == 0 {
		return nil
	}
	keys := make([]string, 0)
	for _, pattern := range patterns {
		matched, err := cache.Keys(pattern)
		if err != nil {
			return err
		}
		keys = append(keys, matched...)
	}
	if len(keys) == 0 {
		return nil
	}
	return cache.DelKeys(keys...)
}

// TemplateKeyBuilder 创建一个基于模板展开的 key 生成器。
func TemplateKeyBuilder(template string) KeyBuilder {
	return func(args []reflect.Value) (string, error) {
		return ExpandKeyTemplate(template, args), nil
	}
}

// ExpandKeyTemplate 把 `{0}`、`{1}` 这类占位符展开为参数值。
func ExpandKeyTemplate(template string, args []reflect.Value) string {
	out := template
	for i, arg := range args {
		out = strings.ReplaceAll(out, "{"+strconv.Itoa(i)+"}", templateArgString(arg))
	}
	return out
}

// Remember 优先从缓存读取值，未命中时调用 loader 并回写缓存。
func Remember[T any](cache Cache, key string, policy Policy, loader func() (T, error)) (T, error) {
	var zero T
	if cache == nil {
		return loader()
	}
	if key != "" {
		if value, err := cache.Get(key); err == nil {
			if cast, ok := value.(T); ok {
				return cast, nil
			}
		} else if !policy.SkipReadErrors {
			return zero, err
		}
	}
	value, err := loader()
	if err != nil {
		return zero, err
	}
	rv := reflect.ValueOf(value)
	if (!rv.IsValid() || rv.IsZero()) && !policy.CacheNil {
		return value, nil
	}
	if key != "" {
		var setErr error
		if policy.TTL > 0 {
			setErr = cache.SetEx(key, value, policy.TTL)
		} else {
			setErr = cache.Set(key, value)
		}
		if setErr != nil && !policy.SkipWriteErrors {
			return zero, setErr
		}
	}
	return value, nil
}

// DecorateE 把函数包装为带缓存读写能力的装饰器版本，并返回错误。
func DecorateE(target any, cache Cache, opts DecoratorOptions) (any, error) {
	typ := reflect.TypeOf(target)
	if typ == nil || typ.Kind() != reflect.Func {
		return nil, fmt.Errorf("cache.Decorate expects a function")
	}
	value := reflect.ValueOf(target)
	keyBuilder := opts.KeyBuilder
	if keyBuilder == nil {
		keyBuilder = DefaultKeyBuilder(opts.Key)
	}
	wrapped := reflect.MakeFunc(typ, func(args []reflect.Value) []reflect.Value {
		key, err := keyBuilder(args)
		if err != nil {
			return failResults(typ, err)
		}
		if key != "" && cache != nil {
			if cached, getErr := cache.Get(key); getErr == nil && cached != nil {
				if values, ok := hydrateCachedResults(typ, cached); ok {
					return values
				}
			} else if getErr != nil && !opts.Policy.SkipReadErrors {
				return failResults(typ, getErr)
			}
		}
		results := value.Call(args)
		if callErr := extractLastError(results); callErr != nil {
			return results
		}
		if len(opts.Invalidate) > 0 && cache != nil {
			_ = cache.DelKeys(opts.Invalidate...)
		}
		if len(opts.InvalidatePatterns) > 0 && cache != nil {
			patterns := make([]string, 0, len(opts.InvalidatePatterns))
			for _, pattern := range opts.InvalidatePatterns {
				patterns = append(patterns, ExpandKeyTemplate(pattern, args))
			}
			_ = InvalidatePattern(cache, patterns...)
		}
		if key != "" && cache != nil && shouldCacheResults(results, opts.Policy.CacheNil) {
			payload := flattenResults(results)
			var setErr error
			if opts.Policy.TTL > 0 {
				setErr = cache.SetJSONEx(key, payload, opts.Policy.TTL)
			} else {
				setErr = cache.SetJSON(key, payload)
			}
			if setErr != nil && !opts.Policy.SkipWriteErrors {
				return failResults(typ, setErr)
			}
		}
		return results
	})
	return wrapped.Interface(), nil
}

// Decorate 把函数包装为带缓存读写能力的装饰器版本。
func Decorate(target any, cache Cache, opts DecoratorOptions) any {
	wrapped, err := DecorateE(target, cache, opts)
	if err != nil {
		return target
	}
	return wrapped
}

func flattenResults(values []reflect.Value) interface{} {
	if len(values) == 2 && isErrorOutput(values[1].Type()) {
		return values[0].Interface()
	}
	out := make([]interface{}, 0, len(values))
	for _, value := range values {
		out = append(out, value.Interface())
	}
	if len(out) == 1 {
		return out[0]
	}
	return out
}

func hydrateCachedResults(typ reflect.Type, cached interface{}) ([]reflect.Value, bool) {
	if typ.NumOut() == 1 {
		rv := reflect.ValueOf(cached)
		if rv.IsValid() && rv.Type().AssignableTo(typ.Out(0)) {
			return []reflect.Value{rv}, true
		}
	}
	var data []byte
	switch value := cached.(type) {
	case []byte:
		data = value
	default:
		var err error
		data, err = json.Marshal(cached)
		if err != nil {
			return nil, false
		}
	}
	results := make([]reflect.Value, typ.NumOut())
	if typ.NumOut() == 2 && isErrorOutput(typ.Out(1)) {
		holder := reflect.New(typ.Out(0))
		if err := json.Unmarshal(data, holder.Interface()); err == nil {
			results[0] = holder.Elem()
			results[1] = reflect.Zero(typ.Out(1))
			return results, true
		}
	}
	return nil, false
}

func failResults(typ reflect.Type, err error) []reflect.Value {
	out := make([]reflect.Value, typ.NumOut())
	for i := 0; i < typ.NumOut(); i++ {
		if isErrorOutput(typ.Out(i)) {
			out[i] = reflect.ValueOf(err)
		} else {
			out[i] = reflect.Zero(typ.Out(i))
		}
	}
	return out
}

func extractLastError(values []reflect.Value) error {
	if len(values) == 0 {
		return nil
	}
	last := values[len(values)-1]
	if !last.IsValid() || !last.Type().Implements(reflect.TypeFor[error]()) || last.IsNil() {
		return nil
	}
	return last.Interface().(error)
}

func shouldCacheResults(results []reflect.Value, cacheNil bool) bool {
	if len(results) == 0 {
		return false
	}
	if len(results) == 1 {
		return cacheNil || !results[0].IsZero()
	}
	return cacheNil || !results[0].IsZero()
}

func isErrorOutput(t reflect.Type) bool {
	return t != nil && t.Implements(reflect.TypeFor[error]())
}

func stringifyArg(value reflect.Value) string {
	if !value.IsValid() {
		return "nil"
	}
	raw := value.Interface()
	if bytes, err := json.Marshal(raw); err == nil {
		return string(bytes)
	}
	return fmt.Sprintf("%v", raw)
}

func templateArgString(value reflect.Value) string {
	if !value.IsValid() {
		return "nil"
	}
	if value.Kind() == reflect.String {
		return value.String()
	}
	return stringifyArg(value)
}
