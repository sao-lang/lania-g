// composite.go 实现 WS composite struct 的组合绑定逻辑。
package ws

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// CompositeStruct 允许把多个 WS wrapper 聚合进一个 struct 参数。
// 例如一个 handler 可以同时拿到 `WSMessageBody[T]`、`Header[T]`、`WsContext`、`WSEvent`。
func matchCompositeStruct(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	original := t
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return runtime.WrapperDescriptor{}, false
	}
	if t.PkgPath() == PackagePath() {
		return runtime.WrapperDescriptor{}, false
	}

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if isSupportedCompositeFieldType(f.Type) {
			return runtime.WrapperDescriptor{Kind: "CompositeStruct", WrapperType: original, InnerType: t}, true
		}
	}
	return runtime.WrapperDescriptor{}, false
}

func resolveCompositeStruct(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	t := desc.WrapperType
	ptr := false
	if t.Kind() == reflect.Ptr {
		ptr = true
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("composite args must be struct or *struct, got %s", desc.WrapperType.String())
	}

	out := reflect.New(t).Elem()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		fv := out.Field(i)
		if !fv.CanSet() {
			continue
		}

		value, ok, err := resolveCompositeField(ctx, f)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		vv := reflect.ValueOf(value)
		if vv.IsValid() && vv.Type().AssignableTo(fv.Type()) {
			fv.Set(vv)
		} else if vv.IsValid() && vv.Type().ConvertibleTo(fv.Type()) {
			fv.Set(vv.Convert(fv.Type()))
		}
	}

	if ptr {
		ptrVal := reflect.New(t)
		ptrVal.Elem().Set(out)
		return ptrVal.Interface(), nil
	}
	return out.Interface(), nil
}

// resolveCompositeField 根据字段类型分发到对应的 WS resolver。
// 它和 HTTP 的 composite 逻辑类似，但支持的 wrapper 更少，重点围绕消息体、header 和 socket 上下文。
func resolveCompositeField(ctx *runtime.HandlerContext, field reflect.StructField) (any, bool, error) {
	if field.Type == reflect.TypeFor[*WsContext]() || field.Type == reflect.TypeFor[Context]() {
		val, err := resolveWsContext(ctx, runtime.WrapperDescriptor{WrapperType: field.Type})
		return val, true, err
	}
	if field.Type == reflect.TypeOf(WSEvent{}) {
		val, err := resolveEvent(ctx, runtime.WrapperDescriptor{WrapperType: field.Type, InnerType: field.Type})
		return val, true, err
	}
	if field.Type == reflect.TypeOf(WSSocketID{}) {
		val, err := resolveSocketID(ctx, runtime.WrapperDescriptor{WrapperType: field.Type, InnerType: field.Type})
		return val, true, err
	}
	if field.Type == reflect.TypeOf(WSRooms{}) {
		val, err := resolveRooms(ctx, runtime.WrapperDescriptor{WrapperType: field.Type, InnerType: field.Type})
		return val, true, err
	}
	if field.Type == reflect.TypeOf(WSConnectedSocket{}) {
		val, err := resolveConnectedSocket(ctx, runtime.WrapperDescriptor{WrapperType: field.Type, InnerType: field.Type})
		return val, true, err
	}
	if field.Type == reflect.TypeOf(Headers(nil)) {
		val, err := resolveHeaders(ctx, runtime.WrapperDescriptor{WrapperType: field.Type, InnerType: field.Type})
		return val, true, err
	}

	ft := field.Type
	if ft.Kind() == reflect.Ptr {
		ft = ft.Elem()
	}
	if ft.Kind() != reflect.Struct || ft.PkgPath() != PackagePath() {
		return nil, false, nil
	}

	name := trimGenericName(ft.Name())
	desc := runtime.WrapperDescriptor{WrapperType: field.Type}
	if vField, ok := ft.FieldByName("Value"); ok {
		desc.InnerType = vField.Type
	}
	required := fieldRequired(field.Tag)

	switch name {
	case "WSMessageBody":
		if key := field.Tag.Get(TagBody); key != "" {
			desc.BindingName = key
		}
		// 没有 bindingName 时表示“整条消息体都解到 Value”；有 bindingName 时只抽单个字段。
		if required && desc.BindingName == "" && len(ctx.Request.BodyBytes) == 0 && ctx.Request.Body == nil {
			return nil, true, fmt.Errorf("missing required body for field %s", field.Name)
		}
		v, err := resolveMessageBody(ctx, desc)
		return v, true, err
	case "Header":
		if key := field.Tag.Get(TagHeader); key != "" {
			desc.BindingName = key
		}
		// Header[T] 在 composite 场景下无法像 HTTP path param 那样自动推导名字，
		// required 时必须显式标记 header tag。
		if required && desc.BindingName == "" {
			return nil, true, fmt.Errorf("required header bindingName missing: add %q tag for field %s", TagHeader, field.Name)
		}
		if required && desc.BindingName != "" && ctx.Request.Headers[desc.BindingName] == "" {
			return nil, true, fmt.Errorf("missing required header %q for field %s", desc.BindingName, field.Name)
		}
		v, err := resolveHeaders(ctx, desc)
		return v, true, err
	}

	return nil, false, nil
}

// isSupportedCompositeFieldType 用来区分：
// - 该 struct 应该走 CompositeStruct
// - 还是普通 AutoStruct 绑定
func isSupportedCompositeFieldType(t reflect.Type) bool {
	if t == reflect.TypeFor[*WsContext]() || t == reflect.TypeFor[Context]() {
		return true
	}
	if t == reflect.TypeOf(WSEvent{}) || t == reflect.TypeOf(WSSocketID{}) || t == reflect.TypeOf(WSRooms{}) || t == reflect.TypeOf(WSConnectedSocket{}) || t == reflect.TypeOf(Headers(nil)) {
		return true
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || t.PkgPath() != PackagePath() {
		return false
	}
	name := trimGenericName(t.Name())
	return name == "WSMessageBody" || name == "Header"
}
