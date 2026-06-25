// matcher.go 负责把“某个 Go 参数类型”识别成对应的 GraphQL binding descriptor。
// 换句话说，resolver 在真正解析值之前，先通过这里判断参数属于哪一种注入语义。
package graphql

import (
	"reflect"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// PackagePath 返回当前 binding 包在运行时看到的 import path。
// 这里不写死字符串，避免目录移动后 matcher 失效。
func PackagePath() string {
	return reflect.TypeOf(OperationName("")).PkgPath()
}

// matchNamedType 用于匹配那些“类型本身就代表 binding 语义”的别名类型。
// 例如 `OperationName`、`FieldName` 这类类型，不需要再额外拆 `Value` 字段。
func matchNamedType[T any](name string) func(reflect.Type) (runtime.WrapperDescriptor, bool) {
	base := reflect.TypeFor[T]()
	return func(t reflect.Type) (runtime.WrapperDescriptor, bool) {
		if t != base {
			return runtime.WrapperDescriptor{}, false
		}
		return runtime.WrapperDescriptor{Kind: name, WrapperType: t, InnerType: t}, true
	}
}

// matchGenericWrapper 处理 `Arg[T]` / `Header[T]` / `Parent[T]` 这种通用 wrapper。
// 识别条件是：
// - 属于本 binding 包
// - 去掉泛型后名字匹配
// - 暴露 `Value` 字段作为真实 inner type
func matchGenericWrapper(baseName string) func(reflect.Type) (runtime.WrapperDescriptor, bool) {
	return func(t reflect.Type) (runtime.WrapperDescriptor, bool) {
		original := t
		if t.Kind() == reflect.Ptr {
			// 匹配阶段允许业务写 `Arg[T]` 或 `*Arg[T]`，统一先下钻。
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct || t.PkgPath() != PackagePath() {
			return runtime.WrapperDescriptor{}, false
		}
		if trimGenericName(t.Name()) != baseName {
			return runtime.WrapperDescriptor{}, false
		}
		field, ok := t.FieldByName("Value")
		if !ok {
			return runtime.WrapperDescriptor{}, false
		}
		return runtime.WrapperDescriptor{Kind: baseName, WrapperType: original, InnerType: field.Type}, true
	}
}

// trimGenericName 去掉 `Foo[T]` 这类运行时类型名中的泛型后缀。
func trimGenericName(name string) string {
	if idx := strings.Index(name, "["); idx >= 0 {
		return name[:idx]
	}
	return name
}

// matchGraphQLContext 同时支持注入 `Context` 接口和 `*GraphQLContext` 具体实现。
func matchGraphQLContext(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	contextType := reflect.TypeFor[Context]()
	ctxPtr := reflect.TypeFor[*GraphQLContext]()
	if t == contextType || t == ctxPtr {
		return runtime.WrapperDescriptor{Kind: "GraphQLContext", WrapperType: t, InnerType: t}, true
	}
	return runtime.WrapperDescriptor{}, false
}

// matchAutoStruct 识别“普通业务输入 struct 自动绑定”场景。
// 如果字段里已经出现显式 wrapper 或专用 tag，就不再视为 AutoStruct，
// 而交给 CompositeStruct 流程处理。
func matchAutoStruct(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	original := t
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || t.PkgPath() == PackagePath() {
		return runtime.WrapperDescriptor{}, false
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		// 一旦发现显式 binding 语义，这个结构体就不再属于“纯自动绑定”。
		if isSupportedCompositeFieldType(f.Type) || f.Tag.Get("arg") != "" || f.Tag.Get("header") != "" {
			return runtime.WrapperDescriptor{}, false
		}
	}
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).IsExported() {
			return runtime.WrapperDescriptor{Kind: "AutoStruct", WrapperType: original, InnerType: t}, true
		}
	}
	return runtime.WrapperDescriptor{}, false
}

// matchCompositeStruct 识别“结构体里混有显式 GraphQL binding 字段”的场景。
// 它和 AutoStruct 的分界线就在于：字段里是否已经出现专用 wrapper 或 tag。
func matchCompositeStruct(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	original := t
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || t.PkgPath() == PackagePath() {
		return runtime.WrapperDescriptor{}, false
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if isSupportedCompositeFieldType(f.Type) || f.Tag.Get("arg") != "" || f.Tag.Get("header") != "" {
			return runtime.WrapperDescriptor{Kind: "CompositeStruct", WrapperType: original, InnerType: t}, true
		}
	}
	return runtime.WrapperDescriptor{}, false
}
