// decode.go 提供 GraphQL 参数与变量值的解码辅助。
package graphql

import (
	"encoding/json"
	"maps"
	"reflect"
	"strings"
)

// decodeTo 是 GraphQL binding 的统一解码入口。
// GraphQL args/variables 往往已经是 map/list/scalar 混合的动态对象，
// 这里统一走“可直接赋值/转换优先，否则 JSON round-trip”策略。
func decodeTo(target reflect.Type, raw any) (reflect.Value, error) {
	if raw == nil {
		return zero(target), nil
	}
	rv := reflect.ValueOf(raw)
	if rv.IsValid() && rv.Type().AssignableTo(target) {
		return rv, nil
	}
	if rv.IsValid() && rv.Type().ConvertibleTo(target) {
		return rv.Convert(target), nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return reflect.Value{}, err
	}
	if target.Kind() == reflect.Ptr {
		out := reflect.New(target.Elem())
		if err := json.Unmarshal(data, out.Interface()); err != nil {
			return reflect.Value{}, err
		}
		return out, nil
	}
	out := reflect.New(target)
	if err := json.Unmarshal(data, out.Interface()); err != nil {
		return reflect.Value{}, err
	}
	return out.Elem(), nil
}

// wrapValue 把解好的 inner value 回填到泛型 wrapper 的 `Value` 字段。
func wrapValue(wrapperType reflect.Type, value reflect.Value) any {
	if wrapperType.Kind() == reflect.Ptr {
		wrapperType = wrapperType.Elem()
	}
	wrapper := reflect.New(wrapperType).Elem()
	field := wrapper.FieldByName("Value")
	if field.IsValid() && field.CanSet() && value.IsValid() {
		if value.Type().AssignableTo(field.Type()) {
			field.Set(value)
		} else if value.Type().ConvertibleTo(field.Type()) {
			field.Set(value.Convert(field.Type()))
		}
	}
	return wrapper.Interface()
}

// toSnakeCase 用作字段名到 GraphQL 参数名的默认推导。
func toSnakeCase(s string) string {
	if len(s) == 0 {
		return s
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && 'A' <= r && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteByte(byte(strings.ToLower(string(r))[0]))
	}
	return b.String()
}

// zero 返回目标类型的零值；指针类型直接返回 nil pointer。
func zero(t reflect.Type) reflect.Value {
	if t.Kind() == reflect.Ptr {
		return reflect.Zero(t)
	}
	return reflect.New(t).Elem()
}

// firstNonEmpty 用于 tag/name 默认值回退。
func firstNonEmpty(items ...string) string {
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			return item
		}
	}
	return ""
}

// copyStringAnyMap 返回 `map[string]any` 的浅拷贝，避免上下文 map 被原地共享修改。
func copyStringAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	maps.Copy(out, m)
	return out
}
