// refbinding.go 提供 integration 侧复用的 wrapper 匹配与封装辅助。
//
// 这些 helper 的目标是避免各个 `integrations/*` 在 binding 层重复实现：
// - “某个 named generic wrapper 怎么匹配”
// - “如何从 marker type 推导名字”
// - “如何把解析值重新包回 wrapper”
package integration

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// MatchNamedWrapper 匹配指定包路径下、指定基础名字的 wrapper struct。
// 典型场景是 `Ref[T]`、`NamedRef[T, Marker]` 这类 integration 自定义包装类型。
func MatchNamedWrapper(pkgPath, baseName string) func(reflect.Type) (runtime.WrapperDescriptor, bool) {
	return func(t reflect.Type) (runtime.WrapperDescriptor, bool) {
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct || t.PkgPath() != pkgPath || t.NumField() == 0 {
			return runtime.WrapperDescriptor{}, false
		}
		name := t.Name()
		if trimmed, _, ok := strings.Cut(name, "["); ok {
			name = trimmed
		}
		if name != baseName {
			return runtime.WrapperDescriptor{}, false
		}
		return runtime.WrapperDescriptor{
			Kind:        baseName,
			WrapperType: t,
			InnerType:   t.Field(0).Type,
		}, true
	}
}

// ResolveMarkerName 从 wrapper 的 marker type 中推导最终名字。
// 这允许 integration 用类型而不是字符串字面量来声明引用名。
func ResolveMarkerName(wrapperType reflect.Type, defaultName string, resolve func(marker any) (string, bool)) string {
	if wrapperType.Kind() == reflect.Ptr {
		wrapperType = wrapperType.Elem()
	}
	if wrapperType.Kind() != reflect.Struct || wrapperType.NumField() < 2 {
		return defaultName
	}
	markerType := wrapperType.Field(wrapperType.NumField() - 1).Type
	if markerType == nil {
		return defaultName
	}
	if markerType.Kind() == reflect.Ptr {
		markerType = markerType.Elem()
	}
	if markerType == nil || markerType.Name() == "" {
		return defaultName
	}
	marker := reflect.New(markerType)
	if marker.CanInterface() {
		if name, ok := resolve(marker.Interface()); ok && name != "" {
			return name
		}
	}
	return strings.ToLower(markerType.Name())
}

// WrapFirstField 把解析出的值写回 wrapper 的第一个字段。
// 这里假设 integration wrapper 的“有效载荷”统一落在第一个字段上。
func WrapFirstField(wrapperType reflect.Type, value reflect.Value) (any, error) {
	if wrapperType.Kind() == reflect.Ptr {
		wrapperType = wrapperType.Elem()
	}
	target := reflect.New(wrapperType).Elem()
	if target.NumField() == 0 {
		return nil, fmt.Errorf("invalid wrapper: %s", wrapperType.String())
	}
	field := target.Field(0)
	if !value.IsValid() {
		return nil, fmt.Errorf("invalid wrapper value for %s", wrapperType.String())
	}
	if !value.Type().AssignableTo(field.Type()) {
		if value.Type().ConvertibleTo(field.Type()) {
			value = value.Convert(field.Type())
		} else {
			return nil, fmt.Errorf("cannot wrap %s into %s", value.Type().String(), field.Type().String())
		}
	}
	field.Set(value)
	return target.Interface(), nil
}
