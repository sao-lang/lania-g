// decode.go 提供 HTTP binding 所需的输入解码辅助。
package http

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
)

// trimGenericName 把 `Body[string]` 这类实例化名字裁成 `Body`，
// 用于 wrapper 识别。
func trimGenericName(name string) string {
	if idx := strings.Index(name, "["); idx >= 0 {
		return name[:idx]
	}
	return name
}

// decodeTo 是 binding/http 的统一解码入口。
// 策略是：
// - `string` 走标量解析
// - `[]byte` 走 JSON 解码
// - 其余对象先看是否可直接赋值/转换，再回退到 JSON marshal+unmarshal
func decodeTo(target reflect.Type, raw any) (reflect.Value, error) {
	if raw == nil {
		return zero(target), nil
	}

	if s, ok := raw.(string); ok {
		return decodeString(target, s)
	}

	if bytes, ok := raw.([]byte); ok {
		return decodeJSON(target, bytes)
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
	return decodeJSON(target, data)
}

// decodeString 负责把单个字符串解到目标类型。
// 对 struct/map/slice 等复杂目标，会把字符串视作 JSON 或 JSON string 再解码。
func decodeString(target reflect.Type, s string) (reflect.Value, error) {
	if strings.TrimSpace(s) == "" {
		return zero(target), nil
	}

	if target.Kind() == reflect.Ptr {
		v, err := decodeString(target.Elem(), s)
		if err != nil {
			return reflect.Value{}, err
		}
		ptr := reflect.New(target.Elem())
		ptr.Elem().Set(v)
		return ptr, nil
	}

	switch target.Kind() {
	case reflect.String:
		return reflect.ValueOf(s).Convert(target), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return reflect.Value{}, err
		}
		v := reflect.New(target).Elem()
		v.SetInt(i)
		return v, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return reflect.Value{}, err
		}
		v := reflect.New(target).Elem()
		v.SetUint(u)
		return v, nil
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return reflect.Value{}, err
		}
		v := reflect.New(target).Elem()
		v.SetFloat(f)
		return v, nil
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return reflect.Value{}, err
		}
		v := reflect.New(target).Elem()
		v.SetBool(b)
		return v, nil
	default:
		trim := strings.TrimSpace(s)
		if strings.HasPrefix(trim, "{") || strings.HasPrefix(trim, "[") {
			// 已经长得像 JSON 对象/数组时，直接按原样解。
			return decodeJSON(target, []byte(trim))
		}
		// 否则把普通字符串包装成 JSON string，再交给 JSON 解码处理命名类型等场景。
		return decodeJSON(target, []byte(strconv.Quote(s)))
	}
}

// decodeJSON 统一承接“目标类型可能是 struct/map/slice/pointer”的 JSON 解码。
func decodeJSON(target reflect.Type, data []byte) (reflect.Value, error) {
	if target.Kind() == reflect.Ptr {
		out := reflect.New(target.Elem())
		if len(data) > 0 {
			if err := json.Unmarshal(data, out.Interface()); err != nil {
				return reflect.Value{}, err
			}
		}
		return out, nil
	}

	out := reflect.New(target)
	if len(data) > 0 {
		if err := json.Unmarshal(data, out.Interface()); err != nil {
			return reflect.Value{}, err
		}
	}
	return out.Elem(), nil
}

// wrapValue 把解好的 inner value 回填到 wrapper 的 `Value` 字段。
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

func zero(t reflect.Type) reflect.Value {
	if t.Kind() == reflect.Ptr {
		return reflect.Zero(t)
	}
	return reflect.New(t).Elem()
}
