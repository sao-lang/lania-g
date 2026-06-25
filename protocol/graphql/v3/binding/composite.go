// composite.go 实现 GraphQL composite struct 的组合绑定逻辑。
package graphql

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// resolveCompositeStruct 处理 GraphQL 的 CompositeStruct 绑定。
// 这里不会把整个 struct 当成单个输入对象整体解码，而是逐字段判断：
// - 若字段本身是已支持的 wrapper/context 类型，就走对应 resolver
// - 若字段带 `arg` / `header` tag，则按显式名字取值
func resolveCompositeStruct(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	t := desc.WrapperType
	ptr := false
	if t.Kind() == reflect.Ptr {
		ptr = true
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("CompositeStruct expects struct/*struct, got %s", desc.WrapperType.String())
	}
	outPtr := reflect.New(t)
	if err := bindCompositeStruct(ctx, outPtr.Elem()); err != nil {
		return nil, err
	}
	if ptr {
		return outPtr.Interface(), nil
	}
	return outPtr.Elem().Interface(), nil
}

// bindSimpleMapToStruct 负责把一个简单 map 里的字段写回普通 struct。
// 它主要服务 GraphQL 参数/变量这类“对象值再嵌套一层 struct”的场景。
func bindSimpleMapToStruct(dst reflect.Value, m map[string]any) error {
	if !dst.IsValid() || dst.Kind() != reflect.Struct {
		return nil
	}
	typ := dst.Type()
	for i := 0; i < dst.NumField(); i++ {
		field := dst.Field(i)
		sf := typ.Field(i)
		if !field.CanSet() || !sf.IsExported() {
			continue
		}
		key := sf.Tag.Get("json")
		if key == "" {
			key = toSnakeCase(sf.Name)
		} else if idx := strings.IndexByte(key, ','); idx >= 0 {
			key = key[:idx]
		}
		val, ok := m[key]
		if !ok {
			continue
		}
		value, err := decodeTo(field.Type(), val)
		if err != nil {
			return err
		}
		assignValue(field, value)
	}
	return nil
}

// bindCompositeStruct 遍历目标 struct 字段，并逐个调用 resolveCompositeField。
func bindCompositeStruct(ctx *runtime.HandlerContext, dst reflect.Value) error {
	typ := dst.Type()
	for i := 0; i < dst.NumField(); i++ {
		field := dst.Field(i)
		sf := typ.Field(i)
		if !sf.IsExported() || !field.CanSet() {
			continue
		}
		value, ok, err := resolveCompositeField(ctx, sf)
		if err != nil {
			return err
		}
		if ok {
			assignValue(field, value)
		}
	}
	return nil
}

// resolveCompositeField 根据字段类型或 tag 选择 GraphQL 绑定来源。
// GraphQL 这里允许两条路径并存：
// - 类型驱动：`Arg[T]`、`Header[T]`、`Context`、`SelectionSet` 等
// - tag 驱动：普通字段上显式写 `arg:"..."` / `header:"..."`
func resolveCompositeField(ctx *runtime.HandlerContext, field reflect.StructField) (reflect.Value, bool, error) {
	t := field.Type
	if isSupportedCompositeFieldType(t) {
		desc, ok := descriptorForField(field)
		if !ok {
			return reflect.Value{}, false, nil
		}
		value, err := resolveByDescriptor(ctx, desc)
		if err != nil {
			return reflect.Value{}, false, err
		}
		return reflect.ValueOf(value), true, nil
	}
	if key := field.Tag.Get("arg"); key != "" {
		args, _ := extractArgs(ctx)
		val, err := decodeTo(t, args[key])
		if err != nil {
			return reflect.Value{}, false, err
		}
		return val, true, nil
	}
	if key := field.Tag.Get("header"); key != "" {
		hdrAny, _ := ctx.Get(MetadataKeyHeaders)
		hdr, _ := hdrAny.(http.Header)
		val, err := decodeTo(t, hdr.Get(key))
		if err != nil {
			return reflect.Value{}, false, err
		}
		return val, true, nil
	}
	return reflect.Value{}, false, nil
}

// descriptorForField 把字段声明翻译成统一的 WrapperDescriptor。
// 这样 composite 绑定后续就能直接复用 resolver.go 里的通用解析流程。
func descriptorForField(field reflect.StructField) (runtime.WrapperDescriptor, bool) {
	t := field.Type
	candidates := []func(reflect.Type) (runtime.WrapperDescriptor, bool){
		matchGraphQLContext,
		matchNamedType[Variables]("Variables"),
		matchNamedType[Headers]("Headers"),
		matchNamedType[Extensions]("Extensions"),
		matchNamedType[SelectionSet]("SelectionSet"),
		matchNamedType[Root]("Root"),
		matchNamedType[Info]("Info"),
		matchNamedType[OperationName]("OperationName"),
		matchNamedType[FieldName]("FieldName"),
		matchNamedType[RawQuery]("RawQuery"),
		matchNamedType[IP]("IP"),
		matchNamedType[Host]("Host"),
		matchNamedType[Method]("Method"),
		matchNamedType[URL]("URL"),
		matchNamedType[Path]("Path"),
		matchNamedType[Session]("Session"),
		matchNamedType[Request]("Request"),
		matchNamedType[Response]("Response"),
	}
	for _, candidate := range candidates {
		if desc, ok := candidate(t); ok {
			return desc, true
		}
	}
	if desc, ok := matchGenericWrapper("Arg")(t); ok {
		desc.BindingName = firstNonEmpty(field.Tag.Get("arg"), toSnakeCase(field.Name))
		return desc, true
	}
	if desc, ok := matchGenericWrapper("ArgValue")(t); ok {
		desc.BindingName = firstNonEmpty(field.Tag.Get("arg"), toSnakeCase(field.Name))
		return desc, true
	}
	if desc, ok := matchGenericWrapper("Header")(t); ok {
		desc.BindingName = firstNonEmpty(field.Tag.Get("header"), field.Name)
		return desc, true
	}
	if desc, ok := matchGenericWrapper("Parent")(t); ok {
		return desc, true
	}
	return runtime.WrapperDescriptor{}, false
}

// isSupportedCompositeFieldType 用来区分：
// - 这个 struct 应该按 CompositeStruct 逐字段解析
// - 还是按 AutoStruct 当普通 DTO 处理
func isSupportedCompositeFieldType(t reflect.Type) bool {
	candidates := []func(reflect.Type) (runtime.WrapperDescriptor, bool){
		matchGraphQLContext,
		matchNamedType[Variables]("Variables"),
		matchNamedType[Headers]("Headers"),
		matchNamedType[Extensions]("Extensions"),
		matchNamedType[SelectionSet]("SelectionSet"),
		matchNamedType[Root]("Root"),
		matchNamedType[Info]("Info"),
		matchNamedType[OperationName]("OperationName"),
		matchNamedType[FieldName]("FieldName"),
		matchNamedType[RawQuery]("RawQuery"),
		matchNamedType[IP]("IP"),
		matchNamedType[Host]("Host"),
		matchNamedType[Method]("Method"),
		matchNamedType[URL]("URL"),
		matchNamedType[Path]("Path"),
		matchNamedType[Session]("Session"),
		matchNamedType[Request]("Request"),
		matchNamedType[Response]("Response"),
	}
	for _, candidate := range candidates {
		if _, ok := candidate(t); ok {
			return true
		}
	}
	for _, name := range []string{"Arg", "ArgValue", "Header", "Parent"} {
		if _, ok := matchGenericWrapper(name)(t); ok {
			return true
		}
	}
	return false
}

// assignValue 封装 Assignable/Convertible 两条回填路径。
func assignValue(dst reflect.Value, value reflect.Value) {
	if !value.IsValid() {
		return
	}
	if value.Type().AssignableTo(dst.Type()) {
		dst.Set(value)
	} else if value.Type().ConvertibleTo(dst.Type()) {
		dst.Set(value.Convert(dst.Type()))
	}
}

// extractArgs 统一读取当前字段的 GraphQL args。
func extractArgs(ctx *runtime.HandlerContext) (map[string]any, bool) {
	v, ok := ctx.Get(MetadataKeyField)
	if !ok {
		return nil, false
	}
	args, ok := v.(map[string]any)
	return args, ok
}

// resolveBoundArg 在没有显式 bindingName 时只允许“刚好一个参数”的安全兜底。
// 多参数场景下强制要求显式声明，避免把绑定结果猜错。
func resolveBoundArg(args map[string]any, bindingName string) (any, error) {
	if bindingName != "" {
		return args[bindingName], nil
	}
	switch len(args) {
	case 0:
		return nil, nil
	case 1:
		for _, value := range args {
			return value, nil
		}
	}
	return nil, fmt.Errorf("ambiguous graphql arg binding: multiple args present, declare FieldBuilder.Arg(...) or use `arg:\\\"...\\\"` tag")
}
