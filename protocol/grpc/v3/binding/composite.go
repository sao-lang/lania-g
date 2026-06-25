// composite.go 实现 gRPC composite struct 的组合绑定逻辑。
package grpc

import (
	stdctx "context"
	"fmt"
	"reflect"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// gRPC composite 目前支持一组轻量 tag：
// - `req`：把当前 unary/server-stream 首请求消息绑定到该字段
// - `header`：按 metadata key 读取单值
// - `required`：要求该字段必须可解析到非空输入
const (
	TagReq      = "req"
	TagHeader   = "header"
	TagRequired = "required"
)

// CompositeStruct 允许把 request/header/context 等能力聚合进一个 struct 参数。
// 它的目标不是替代原生 gRPC handler 签名，而是给“想把入参收口成一个 DTO”的场景
// 提供与 HTTP/WS/GraphQL 更一致的体验。
func matchCompositeStruct(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	original := t
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return runtime.WrapperDescriptor{}, false
	}
	// 本包自己的 wrapper 不是 composite 容器，避免递归/误匹配。
	if t.PkgPath() == PackagePath() {
		return runtime.WrapperDescriptor{}, false
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		if hasCompositeBindingTag(field.Tag) || isSupportedCompositeFieldType(field.Type) {
			return runtime.WrapperDescriptor{
				Kind:        "CompositeStruct",
				WrapperType: original,
				InnerType:   t,
			}, true
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
		return nil, fmt.Errorf("grpc composite args must be struct or *struct, got %s", desc.WrapperType.String())
	}

	out := reflect.New(t).Elem()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		dst := out.Field(i)
		if !dst.CanSet() {
			continue
		}

		value, ok, err := resolveCompositeField(ctx, field)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		rv := reflect.ValueOf(value)
		if rv.IsValid() && rv.Type().AssignableTo(dst.Type()) {
			dst.Set(rv)
		} else if rv.IsValid() && rv.Type().ConvertibleTo(dst.Type()) {
			dst.Set(rv.Convert(dst.Type()))
		}
	}

	if ptr {
		outPtr := reflect.New(t)
		outPtr.Elem().Set(out)
		return outPtr.Interface(), nil
	}
	return out.Interface(), nil
}

// resolveCompositeField 同时支持两条路径：
// - 类型驱动：`Req[T]`、`Header[T]`、`Metadata`、`context.Context` 等
// - tag 驱动：普通字段显式写 `req:"true"` / `header:"Authorization"`
func resolveCompositeField(ctx *runtime.HandlerContext, field reflect.StructField) (any, bool, error) {
	if field.Type == reflect.TypeFor[*runtime.HandlerContext]() {
		return ctx, true, nil
	}
	if _, ok := matchGRPCContext(field.Type); ok {
		value, err := resolveGRPCContext(ctx, runtime.WrapperDescriptor{WrapperType: field.Type, InnerType: field.Type})
		return value, true, err
	}
	if field.Type == reflect.TypeFor[stdctx.Context]() {
		value, err := resolveStdContext(ctx, runtime.WrapperDescriptor{WrapperType: field.Type, InnerType: field.Type})
		return value, true, err
	}
	if field.Type == reflect.TypeOf(Metadata(nil)) {
		value, err := resolveIncomingMetadata(ctx, runtime.WrapperDescriptor{WrapperType: field.Type, InnerType: field.Type})
		return value, true, err
	}
	if field.Type == reflect.TypeOf(FullMethod("")) {
		value, err := resolveFullMethod(ctx, runtime.WrapperDescriptor{WrapperType: field.Type, InnerType: field.Type})
		return value, true, err
	}
	if field.Type == reflect.TypeOf(Service("")) {
		value, err := resolveService(ctx, runtime.WrapperDescriptor{WrapperType: field.Type, InnerType: field.Type})
		return value, true, err
	}
	if field.Type == reflect.TypeOf(Method("")) {
		value, err := resolveMethod(ctx, runtime.WrapperDescriptor{WrapperType: field.Type, InnerType: field.Type})
		return value, true, err
	}

	ft := field.Type
	if ft.Kind() == reflect.Ptr {
		ft = ft.Elem()
	}
	required := fieldRequired(field.Tag)
	if ft.Kind() == reflect.Struct && ft.PkgPath() == PackagePath() {
		name := trimGenericName(ft.Name())
		desc := runtime.WrapperDescriptor{WrapperType: field.Type}
		if vField, ok := ft.FieldByName("Value"); ok {
			desc.InnerType = vField.Type
		}

		switch name {
		case "Req":
			if required && !hasRequestBody(ctx) {
				return nil, true, fmt.Errorf("missing required request message for field %s", field.Name)
			}
			value, err := resolveReqWrapper(ctx, desc)
			return value, true, err
		case "Header":
			if key := field.Tag.Get(TagHeader); key != "" {
				desc.BindingName = key
			}
			if required && desc.BindingName == "" {
				return nil, true, fmt.Errorf("required grpc header binding name missing: add %q tag for field %s", TagHeader, field.Name)
			}
			if required && !hasIncomingMetadataValue(ctx, desc.BindingName) {
				return nil, true, fmt.Errorf("missing required grpc header %q for field %s", desc.BindingName, field.Name)
			}
			value, err := resolveHeader(ctx, desc)
			return value, true, err
		case "RawServerStream":
			value, err := resolveRawServerStream(ctx, desc)
			return value, true, err
		case "ServerStream":
			value, err := resolveServerStream(ctx, desc)
			return value, true, err
		case "ClientStream":
			value, err := resolveClientStream(ctx, desc)
			return value, true, err
		case "BidiStream":
			value, err := resolveBidiStream(ctx, desc)
			return value, true, err
		}
	}

	if field.Tag.Get(TagReq) != "" {
		value, err := resolveCompositeReqField(ctx, field.Type, field.Name, required)
		return value, true, err
	}
	if key := field.Tag.Get(TagHeader); key != "" {
		value, err := resolveCompositeHeaderField(ctx, field.Type, field.Name, key, required)
		return value, true, err
	}
	return nil, false, nil
}

func resolveCompositeReqField(ctx *runtime.HandlerContext, target reflect.Type, fieldName string, required bool) (any, error) {
	if err := ensureModeAllowed(ctx, "CompositeStruct req", "unary", "server_stream"); err != nil {
		return nil, err
	}
	if required && !hasRequestBody(ctx) {
		return nil, fmt.Errorf("missing required request message for field %s", fieldName)
	}
	raw := any(nil)
	if ctx != nil && ctx.Request != nil {
		raw = ctx.Request.Body
	}
	value, err := decodeTo(target, raw)
	if err != nil {
		return nil, err
	}
	return value.Interface(), nil
}

func resolveCompositeHeaderField(ctx *runtime.HandlerContext, target reflect.Type, fieldName string, key string, required bool) (any, error) {
	if required && !hasIncomingMetadataValue(ctx, key) {
		return nil, fmt.Errorf("missing required grpc header %q for field %s", key, fieldName)
	}
	value, err := decodeTo(target, firstIncomingMetadataValue(ctx, key))
	if err != nil {
		return nil, err
	}
	return value.Interface(), nil
}

func isSupportedCompositeFieldType(t reflect.Type) bool {
	if t == reflect.TypeFor[*runtime.HandlerContext]() ||
		t == reflect.TypeFor[stdctx.Context]() ||
		t == reflect.TypeOf(Metadata(nil)) ||
		t == reflect.TypeOf(FullMethod("")) ||
		t == reflect.TypeOf(Service("")) ||
		t == reflect.TypeOf(Method("")) {
		return true
	}
	if _, ok := matchGenericWrapper("Req")(t); ok {
		return true
	}
	if _, ok := matchGenericWrapper("Header")(t); ok {
		return true
	}
	if _, ok := matchGRPCContext(t); ok {
		return true
	}
	if _, ok := matchRawServerStream(t); ok {
		return true
	}
	if _, ok := matchServerStream(t); ok {
		return true
	}
	if _, ok := matchClientStream(t); ok {
		return true
	}
	if _, ok := matchBidiStream(t); ok {
		return true
	}
	return false
}

func hasCompositeBindingTag(tag reflect.StructTag) bool {
	return tag.Get(TagReq) != "" || tag.Get(TagHeader) != ""
}

func fieldRequired(tag reflect.StructTag) bool {
	raw := strings.ToLower(strings.TrimSpace(tag.Get(TagRequired)))
	switch raw {
	case "", "0", "false", "no":
		return false
	default:
		return true
	}
}

func hasRequestBody(ctx *runtime.HandlerContext) bool {
	return ctx != nil && ctx.Request != nil && ctx.Request.Body != nil
}

func hasIncomingMetadataValue(ctx *runtime.HandlerContext, key string) bool {
	if strings.TrimSpace(key) == "" {
		return false
	}
	return firstIncomingMetadataValue(ctx, key) != ""
}

func firstIncomingMetadataValue(ctx *runtime.HandlerContext, key string) string {
	if strings.TrimSpace(key) == "" {
		return ""
	}
	md := incomingMetadata(ctx)
	values := md.Get(strings.ToLower(key))
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
