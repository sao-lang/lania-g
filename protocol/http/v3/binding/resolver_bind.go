// resolver_bind.go 实现 HTTP Bind 与 MustBind 包装类型的解析逻辑。
package http

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// resolveAutoStruct 处理“普通业务 struct 直接按请求数据填充”的兜底场景。
// 它和 CompositeStruct 的边界是：这里假设字段本身不再携带特殊 binding 语义，
// 整个对象只需要走一次统一的 BindInto。
func resolveAutoStruct(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	t := desc.WrapperType
	ptr := false
	if t.Kind() == reflect.Ptr {
		ptr = true
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("AutoStruct expects struct/*struct, got %s", desc.WrapperType.String())
	}

	outPtr := reflect.New(t)
	if err := BindInto(ctx, outPtr.Interface()); err != nil {
		return nil, err
	}

	// validator 是 adapter 在请求入口按需注入的可选能力。
	// 这里放在 BindInto 之后执行，保证校验面对的是已经完成字段映射的对象。
	if vAny, ok := ctx.Get(MetadataKeyValidator); ok && vAny != nil {
		if v, ok := vAny.(Validator); ok && v != nil {
			if err := v.Validate(outPtr.Interface()); err != nil {
				return nil, err
			}
		}
	}

	if ptr {
		return outPtr.Interface(), nil
	}
	return outPtr.Elem().Interface(), nil
}

func resolveBody(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	raw := any(ctx.Request.Body)
	if len(ctx.Request.BodyBytes) > 0 {
		// 若 adapter 预读了 body，就优先用 bodyBytes，避免重复读 io.Reader。
		raw = ctx.Request.BodyBytes
	}
	value, err := decodeTo(desc.InnerType, raw)
	if err != nil {
		return nil, err
	}
	return wrapValue(desc.WrapperType, value), nil
}

func resolveMustBody(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	// Must 语义只额外收紧“不能为空”；实际解码流程仍复用 resolveBody。
	if len(ctx.Request.BodyBytes) == 0 && ctx.Request.Body == nil {
		return nil, fmt.Errorf("missing request body")
	}
	return resolveBody(ctx, desc)
}

func resolveBind(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	value, err := buildBoundValue(ctx, desc.InnerType, false)
	if err != nil {
		return nil, err
	}
	return wrapValue(desc.WrapperType, value), nil
}

func resolveMustBind(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	value, err := buildBoundValue(ctx, desc.InnerType, true)
	if err != nil {
		return nil, err
	}
	return wrapValue(desc.WrapperType, value), nil
}

// buildBoundValue 支撑 `Bind[T]` / `MustBind[T]`：
// - T 必须是本包 wrapper（Body/Param/Query/Header）
// - wrapper 的 `Value` 必须是 struct 或 *struct
// - 先对 `Value` 指向的业务对象做严格绑定，再把结果回填到 wrapper
func buildBoundValue(ctx *runtime.HandlerContext, inner reflect.Type, must bool) (reflect.Value, error) {
	if inner.Kind() == reflect.Ptr {
		inner = inner.Elem()
	}
	if inner.Kind() != reflect.Struct || inner.PkgPath() != PackagePath() {
		return reflect.Value{}, fmt.Errorf("Bind[T] only supports http wrappers Body/Param/Query/Header, got %s", inner.String())
	}

	wrapperName := trimGenericName(inner.Name())
	if wrapperName != "Body" && wrapperName != "Param" && wrapperName != "Query" && wrapperName != "Header" {
		return reflect.Value{}, fmt.Errorf("Bind[T] only supports Body/Param/Query/Header, got %s", wrapperName)
	}

	valueField, ok := inner.FieldByName("Value")
	if !ok {
		return reflect.Value{}, fmt.Errorf("invalid wrapper %s: missing Value field", inner.String())
	}
	uType := valueField.Type

	uPtr := false
	if uType.Kind() == reflect.Ptr {
		uPtr = true
		uType = uType.Elem()
	}
	if uType.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("Bind[T] expects wrapper Value to be struct/*struct, got %s", valueField.Type.String())
	}

	uValPtr := reflect.New(uType)
	// BindStrictInto 比 AutoStruct 更严格：会把无法识别/无法映射的输入当成错误，
	// 这样 `Bind[T]` 更接近“声明式输入 DTO”的预期。
	if err := BindStrictInto(ctx, uValPtr.Interface()); err != nil {
		return reflect.Value{}, err
	}

	if must && wrapperName == "Body" && len(ctx.Request.BodyBytes) == 0 && ctx.Request.Body == nil {
		return reflect.Value{}, fmt.Errorf("missing request body")
	}

	if vAny, ok := ctx.Get(MetadataKeyValidator); ok && vAny != nil {
		if v, ok := vAny.(Validator); ok {
			if err := v.Validate(uValPtr.Interface()); err != nil {
				return reflect.Value{}, err
			}
		}
	}

	var uValue reflect.Value
	if uPtr {
		uValue = uValPtr
	} else {
		uValue = uValPtr.Elem()
	}

	wrapperVal := reflect.New(inner).Elem()
	field := wrapperVal.FieldByName("Value")
	if uValue.IsValid() && field.IsValid() && field.CanSet() {
		// 这里允许 Assignable/Convertible 两种路径，
		// 兼容命名类型、别名类型等“底层结构一致”的场景。
		if uValue.Type().AssignableTo(field.Type()) {
			field.Set(uValue)
		} else if uValue.Type().ConvertibleTo(field.Type()) {
			field.Set(uValue.Convert(field.Type()))
		}
	}
	return wrapperVal, nil
}
