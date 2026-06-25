// matcher.go 提供 HTTP binding wrapper 的运行时匹配辅助。
package http

import (
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// PackagePath returns the runtime import path for this binding package.
//
// 这里通过真实类型反射包路径，而不是写死字符串，
// 这样在 module rename / fork / replace 的场景下仍能正确识别“是不是本包 wrapper”。
func PackagePath() string {
	return reflect.TypeOf(BodyBytes("")).PkgPath()
}

// matchHandlerContext 允许 handler 直接注入 runtime.HandlerContext 或其值拷贝。
// 值类型注入时会在 resolve 阶段构造一个浅拷贝，避免调用方误以为可以原地修改共享上下文。
func matchHandlerContext(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	base := reflect.TypeOf(runtime.HandlerContext{})
	if t == base || t == reflect.PointerTo(base) {
		return runtime.WrapperDescriptor{Kind: "context", WrapperType: t, InnerType: t}, true
	}
	return runtime.WrapperDescriptor{}, false
}

func resolveHandlerContext(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if desc.WrapperType.Kind() == reflect.Ptr {
		return ctx, nil
	}
	// 值类型只复制当前请求可见的上下文字段，不复制内部引用计数等运行时状态。
	return runtime.HandlerContext{
		Protocol:  ctx.Protocol,
		RouteKey:  ctx.RouteKey,
		Container: ctx.Container,
		Request:   ctx.Request,
		Response:  ctx.Response,
		Metadata:  ctx.Metadata,
	}, nil
}

// matchHTTPContext 匹配 binding/http 暴露的 Context 抽象或其具体实现 `*HttpContext`。
// 这样业务层既可以依赖接口，也可以在确有需要时拿到底层扩展能力。
func matchHTTPContext(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	contextType := reflect.TypeFor[Context]()
	httpContextPtr := reflect.TypeFor[*HttpContext]()
	if t == contextType || t == httpContextPtr {
		return runtime.WrapperDescriptor{Kind: "HttpContext", WrapperType: t, InnerType: t}, true
	}
	return runtime.WrapperDescriptor{}, false
}

func resolveHTTPContext(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if existing, ok := ctx.Get(MetadataKeyContext); ok {
		if hc, ok := existing.(*HttpContext); ok && hc != nil {
			if desc.WrapperType.Kind() == reflect.Interface {
				return Context(hc), nil
			}
			return hc, nil
		}
	}

	// HttpContext 只在单次请求里构造一次，后续统一从 metadata 复用，
	// 避免重复解析 request/response 包装对象。
	hc, err := NewHttpContext(ctx)
	if err != nil {
		return nil, err
	}
	ctx.Set(MetadataKeyContext, hc)
	if desc.WrapperType.Kind() == reflect.Interface {
		return Context(hc), nil
	}
	return hc, nil
}

func matchNamedType[T any](name string) func(reflect.Type) (runtime.WrapperDescriptor, bool) {
	base := reflect.TypeFor[T]()
	return func(t reflect.Type) (runtime.WrapperDescriptor, bool) {
		if t != base {
			return runtime.WrapperDescriptor{}, false
		}
		return runtime.WrapperDescriptor{Kind: name, WrapperType: t, InnerType: t}, true
	}
}

func matchGenericWrapper(baseName string) func(reflect.Type) (runtime.WrapperDescriptor, bool) {
	return func(t reflect.Type) (runtime.WrapperDescriptor, bool) {
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		// 只识别本包内的泛型 wrapper，避免把业务自定义 struct 错当成绑定包装类型。
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
		return runtime.WrapperDescriptor{Kind: baseName, WrapperType: t, InnerType: field.Type}, true
	}
}

// matchAutoStruct 负责“普通 struct 自动绑定”的最后兜底。
// 一旦字段里出现已支持的 composite wrapper，就把机会让给 CompositeStruct，
// 避免一半字段按自动绑定、一半字段按特殊 wrapper 的语义混杂。
func matchAutoStruct(t reflect.Type) (runtime.WrapperDescriptor, bool) {
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
			return runtime.WrapperDescriptor{}, false
		}
	}

	hasExported := false
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).IsExported() {
			hasExported = true
			break
		}
	}
	if !hasExported {
		return runtime.WrapperDescriptor{}, false
	}
	return runtime.WrapperDescriptor{Kind: "AutoStruct", WrapperType: original, InnerType: t}, true
}
