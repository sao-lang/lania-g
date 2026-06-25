// moduleref_typed.go 提供 `ModuleRef` 的泛型便捷读取接口。
//
// 它不增加新的能力，只是把常见的 `reflect.Type token + type assertion`
// 收敛成更易用的泛型 API。
package module

import (
	"fmt"
	"reflect"
)

// GetByType 是 ModuleRef 的泛型便捷方法。
//
// 它会使用 `reflect.TypeFor[T]()` 作为 token，
// 再从 root container 中解析对应实例。
func GetByType[T any](ref *ModuleRef) (T, error) {
	var zero T
	if ref == nil {
		return zero, fmt.Errorf("module ref is nil")
	}
	value, err := ref.Get(typeToken[T]())
	if err != nil {
		return zero, err
	}
	return value.(T), nil
}

// MustGetByType 是 GetByType 的 panic 版本。
// 适合测试或启动期“失败就应该中止”的场景。
func MustGetByType[T any](ref *ModuleRef) T {
	value, err := GetByType[T](ref)
	if err != nil {
		panic(err)
	}
	return value
}

// typeToken 返回泛型类型 T 对应的 reflect.Type token。
// 这里统一用 `(*T)(nil)).Elem()`，兼容接口类型和具体类型两种用法。
func typeToken[T any]() reflect.Type {
	return reflect.TypeFor[T]()
}
