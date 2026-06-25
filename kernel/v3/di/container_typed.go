package di

import (
	"fmt"
	"reflect"
)

// GetByType/MustGetByType 是基于泛型的便捷方法：
// token 统一使用 `reflect.TypeFor[T]()`，与 container 中常见的 type token 风格一致。
func GetByType[T any](c *Container) (T, error) {
	var zero T
	if c == nil {
		return zero, fmt.Errorf("container is nil")
	}
	value, err := c.Get(typeToken[T]())
	if err != nil {
		return zero, err
	}
	return value.(T), nil
}

// MustGetByType 是 GetByType 的 panic 版本：解析失败会 panic。
func MustGetByType[T any](c *Container) T {
	value, err := GetByType[T](c)
	if err != nil {
		panic(err)
	}
	return value
}

// typeToken 返回泛型类型 T 对应的 reflect.Type token。
//
// token 形态为：`reflect.TypeFor[T]()`，这是 Go 中常见的“接口/类型 token”写法。
func typeToken[T any]() reflect.Type {
	return reflect.TypeFor[T]()
}
