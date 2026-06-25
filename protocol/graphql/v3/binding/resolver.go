// resolver.go 实现 GraphQL binding 的具体解析流程：
// - 从 HandlerContext metadata 中取出 GraphQL 请求期数据
// - 按 wrapper descriptor 决定读取哪一类运行时值
// - 必要时把原始值解码成目标类型，再回填到 wrapper
package graphql

import (
	"fmt"
	"net"
	"net/http"
	"reflect"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

func resolveGraphQLContext(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyContext); ok {
		if gc, ok := v.(*GraphQLContext); ok && gc != nil {
			// `Context` 是接口注入场景；这里显式转成接口类型，
			// 让 handler 能以抽象上下文而不是具体实现接收它。
			if desc.WrapperType.Kind() == reflect.Interface {
				return Context(gc), nil
			}
			return gc, nil
		}
	}
	// 这里和 HTTP/WS 一样采用“缺值返回零值”策略，而不是直接报错，
	// 这样可让 binding registry 继续按常规零值语义执行。
	return (*GraphQLContext)(nil), nil
}

// resolveFromArgs 处理 `Arg[T]` / `ArgValue[T]` 这类 wrapper。
// 流程是：先按 bindingName 取原始参数值，再解码到目标 inner type，最后回填 wrapper。
func resolveFromArgs(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	args, _ := extractArgs(ctx)
	raw, err := resolveBoundArg(args, desc.BindingName)
	if err != nil {
		return nil, err
	}
	value, err := decodeTo(desc.InnerType, raw)
	if err != nil {
		return nil, err
	}
	return wrapValue(desc.WrapperType, value), nil
}

// resolveHeader 既支持 `Header[string]` 这种“按名取单值”，
// 也支持没有 bindingName 时把整份 header map 解码给目标类型。
func resolveHeader(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	var raw any
	if v, ok := ctx.Get(MetadataKeyHeaders); ok {
		if h, ok2 := v.(http.Header); ok2 {
			if desc.BindingName != "" {
				raw = h.Get(desc.BindingName)
			} else {
				raw = h
			}
		}
	}
	val, err := decodeTo(desc.InnerType, raw)
	if err != nil {
		return nil, err
	}
	return wrapValue(desc.WrapperType, val), nil
}

// resolveParent 读取 GraphQL resolver 的父节点值（root/parent）。
// 这个值通常来自上一级字段执行结果，随后按目标 inner type 再做一次解码。
func resolveParent(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	var raw any
	if v, ok := ctx.Get(MetadataKeyRoot); ok {
		raw = v
	}
	value, err := decodeTo(desc.InnerType, raw)
	if err != nil {
		return nil, err
	}
	return wrapValue(desc.WrapperType, value), nil
}

func resolveVariables(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyVars); ok {
		if m, ok2 := v.(map[string]any); ok2 {
			return Variables(copyStringAnyMap(m)), nil
		}
	}
	return Variables(nil), nil
}

func resolveHeadersMap(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyHeaders); ok {
		if h, ok2 := v.(http.Header); ok2 {
			out := Headers{}
			for k, values := range h {
				out[k] = append([]string{}, values...)
			}
			return out, nil
		}
	}
	return Headers(nil), nil
}

func resolveExtensions(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyExtensions); ok {
		if m, ok2 := v.(map[string]any); ok2 {
			return Extensions(copyStringAnyMap(m)), nil
		}
	}
	return Extensions(nil), nil
}

func resolveSelectionSet(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeySelectionSet); ok {
		if set, ok2 := v.(*SelectionSet); ok2 && set != nil {
			return *set, nil
		}
		if set, ok2 := v.(SelectionSet); ok2 {
			return set, nil
		}
	}
	return SelectionSet{}, nil
}

func resolveRoot(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyRoot); ok {
		return Root(v), nil
	}
	return Root(nil), nil
}

func resolveInfo(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyInfo); ok {
		return Info(v), nil
	}
	return Info(nil), nil
}

func resolveOperationName(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyOperationName); ok {
		if name, ok2 := v.(string); ok2 {
			return OperationName(name), nil
		}
	}
	return OperationName(""), nil
}

func resolveFieldName(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyFieldName); ok {
		if name, ok2 := v.(string); ok2 {
			return FieldName(name), nil
		}
	}
	return FieldName(""), nil
}

func resolveRawQuery(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyRawQuery); ok {
		if query, ok2 := v.(string); ok2 {
			return RawQuery(query), nil
		}
	}
	return RawQuery(""), nil
}

func resolveIP(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if req, ok := ctx.Request.Raw.(*http.Request); ok && req != nil {
		xff := req.Header.Get("X-Forwarded-For")
		// 只取代理链中的第一个地址，保持与常见“client IP”语义一致。
		if idx := strings.IndexByte(xff, ','); idx >= 0 {
			xff = xff[:idx]
		}
		if xff != "" {
			return IP(strings.TrimSpace(xff)), nil
		}
		// 如果没有代理头，再回退到底层连接地址。
		if host, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
			return IP(host), nil
		}
	}
	return IP(""), nil
}

func resolveHost(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if req, ok := ctx.Request.Raw.(*http.Request); ok && req != nil {
		return Host(req.Host), nil
	}
	return Host(""), nil
}

func resolveMethod(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if req, ok := ctx.Request.Raw.(*http.Request); ok && req != nil {
		return Method(req.Method), nil
	}
	return Method(""), nil
}

func resolveURL(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if req, ok := ctx.Request.Raw.(*http.Request); ok && req != nil && req.URL != nil {
		return URL(req.URL.String()), nil
	}
	return URL(""), nil
}

func resolvePath(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	return Path(ctx.Request.Path), nil
}

func resolveSession(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeySession); ok {
		if m, ok2 := v.(map[string]any); ok2 {
			return Session(copyStringAnyMap(m)), nil
		}
	}
	return Session(nil), nil
}

func resolveRequest(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if req, ok := ctx.Request.Raw.(*http.Request); ok {
		return Request(req), nil
	}
	return Request(nil), nil
}

func resolveResponse(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if rw, ok := ctx.Response.Raw.(http.ResponseWriter); ok {
		return Response(rw), nil
	}
	return Response(nil), nil
}

// resolveAutoStruct 处理“普通业务 struct 自动按参数名填充”的兜底场景。
// 它和 CompositeStruct 的区别在于：这里要求字段本身不再携带特殊 binding 语义，
// 而是把整个 args map 视为一个简单输入对象进行绑定。
func resolveAutoStruct(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	t := desc.WrapperType
	ptr := false
	if t.Kind() == reflect.Ptr {
		ptr = true
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, nil
	}
	outPtr := reflect.New(t)
	if args, ok := extractArgs(ctx); ok {
		// 绑定时直接写入 struct 值；是否返回指针由原始 wrapperType 决定。
		if err := bindSimpleMapToStruct(outPtr.Elem(), args); err != nil {
			return nil, err
		}
	}
	if ptr {
		return outPtr.Interface(), nil
	}
	return outPtr.Elem().Interface(), nil
}

// resolveByDescriptor 是 composite struct 绑定的分发入口。
// 当 composite 字段已经在 matcher 阶段被识别为某种 Kind 时，
// 这里统一根据 Kind 路由到对应的细分 resolver。
func resolveByDescriptor(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	switch desc.Kind {
	case "GraphQLContext":
		return resolveGraphQLContext(ctx, desc)
	case "Arg", "ArgValue":
		return resolveFromArgs(ctx, desc)
	case "Header":
		return resolveHeader(ctx, desc)
	case "Parent":
		return resolveParent(ctx, desc)
	case "Variables":
		return resolveVariables(ctx, desc)
	case "Headers":
		return resolveHeadersMap(ctx, desc)
	case "Extensions":
		return resolveExtensions(ctx, desc)
	case "SelectionSet":
		return resolveSelectionSet(ctx, desc)
	case "Root":
		return resolveRoot(ctx, desc)
	case "Info":
		return resolveInfo(ctx, desc)
	case "OperationName":
		return resolveOperationName(ctx, desc)
	case "FieldName":
		return resolveFieldName(ctx, desc)
	case "RawQuery":
		return resolveRawQuery(ctx, desc)
	case "IP":
		return resolveIP(ctx, desc)
	case "Host":
		return resolveHost(ctx, desc)
	case "Method":
		return resolveMethod(ctx, desc)
	case "URL":
		return resolveURL(ctx, desc)
	case "Path":
		return resolvePath(ctx, desc)
	case "Session":
		return resolveSession(ctx, desc)
	case "Request":
		return resolveRequest(ctx, desc)
	case "Response":
		return resolveResponse(ctx, desc)
	default:
		return nil, fmt.Errorf("unsupported graphql composite field kind %q", desc.Kind)
	}
}
