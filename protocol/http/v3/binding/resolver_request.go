// resolver_request.go 实现 HTTP 请求对象与元数据相关 binding 的解析逻辑。
package http

import (
	"maps"
	"net/http"
	"reflect"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

func resolveQuery(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	raw := any(ctx.Request.Query)
	if desc.BindingName != "" {
		// 指定 binding name 时退化为“取单个 query 参数”的语义；
		// 不指定时才把整个 query map 交给 decodeTo。
		raw = ctx.Request.Query[desc.BindingName]
	}
	value, err := decodeTo(desc.InnerType, raw)
	if err != nil {
		return nil, err
	}
	return wrapValue(desc.WrapperType, value), nil
}

func resolveForm(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	raw := any(map[string][]string(nil))
	if v, ok := ctx.Get(MetadataKeyForm); ok {
		if m, ok := v.(map[string][]string); ok {
			raw = m
			if desc.BindingName != "" {
				// form 单字段默认取第一个值，与常见 `application/x-www-form-urlencoded`
				// 的“一个字段对应一个主值”习惯保持一致。
				if list := m[desc.BindingName]; len(list) > 0 {
					raw = list[0]
				} else {
					raw = ""
				}
			}
		}
	}
	value, err := decodeTo(desc.InnerType, raw)
	if err != nil {
		return nil, err
	}
	return wrapValue(desc.WrapperType, value), nil
}

func resolveParam(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if ctx == nil || ctx.Request == nil {
		// 路由参数缺失时返回零值，而不是报错；
		// 是否必须存在，由业务校验或 Must/validator 语义决定。
		return wrapValue(desc.WrapperType, zero(desc.InnerType)), nil
	}
	if desc.BindingName != "" {
		value, err := decodeTo(desc.InnerType, ctx.Request.Params[desc.BindingName])
		if err != nil {
			return nil, err
		}
		return wrapValue(desc.WrapperType, value), nil
	}
	value, err := decodeTo(desc.InnerType, ctx.Request.Params)
	if err != nil {
		return nil, err
	}
	return wrapValue(desc.WrapperType, value), nil
}

func resolveHeader(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	raw := any(ctx.Request.Headers)
	if desc.BindingName != "" {
		raw = ctx.Request.Headers[desc.BindingName]
	}
	value, err := decodeTo(desc.InnerType, raw)
	if err != nil {
		return nil, err
	}
	return wrapValue(desc.WrapperType, value), nil
}

func resolveBodyBytes(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if desc.WrapperType == reflect.TypeOf(BodyBytes(nil)) {
		return BodyBytes(ctx.Request.BodyBytes), nil
	}
	if desc.WrapperType == reflect.TypeOf(MustBodyBytes(nil)) {
		return MustBodyBytes(ctx.Request.BodyBytes), nil
	}
	return reflect.Zero(desc.WrapperType).Interface(), nil
}

func resolveOriginal(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if req, ok := ctx.Request.Raw.(*http.Request); ok {
		return Original(req), nil
	}
	return Original(nil), nil
}

func resolveNext(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if nextAny, ok := ctx.Get(MetadataKeyNext); ok {
		if fn, ok := nextAny.(func() error); ok {
			return Next(fn), nil
		}
		if fn, ok := nextAny.(Next); ok {
			return fn, nil
		}
	}
	// 没有下游时提供 no-op，避免 handler 每次都要判空。
	return Next(func() error { return nil }), nil
}

func resolveIP(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	xff := ctx.Request.Headers["X-Forwarded-For"]
	// 若经过多层代理，只取首个地址作为最原始客户端 IP。
	if idx := strings.IndexByte(xff, ','); idx >= 0 {
		xff = xff[:idx]
	}
	return IP(strings.TrimSpace(xff)), nil
}

func resolveHost(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	return Host(ctx.Request.Headers["Host"]), nil
}

func resolveMethod(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	return Method(ctx.Request.Method), nil
}

func resolvePath(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	return Path(ctx.Request.Path), nil
}

func resolveURL(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if req, ok := ctx.Request.Raw.(*http.Request); ok && req != nil && req.URL != nil {
		return URL(req.URL.String()), nil
	}
	return URL(ctx.Request.Path), nil
}

func resolveHeaders(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	out := make(map[string][]string, len(ctx.Request.HeadersMulti))
	for k, v := range ctx.Request.HeadersMulti {
		// 返回快照，避免业务层直接改动 runtime 内部 header 存储。
		out[k] = append([]string{}, v...)
	}
	return Headers(out), nil
}

func resolveSession(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if sessAny, ok := ctx.Get(MetadataKeySession); ok {
		if m, ok := sessAny.(map[string]any); ok {
			return Session(m), nil
		}
	}
	return Session(nil), nil
}

func resolveFile(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	filesAny, _ := ctx.Get(MetadataKeyFiles)
	files, _ := filesAny.(map[string][]*UploadedFile)
	if len(files) == 0 {
		return File{Value: nil}, nil
	}
	if desc.BindingName != "" {
		if list := files[desc.BindingName]; len(list) > 0 {
			return File{Value: list[0]}, nil
		}
		return File{Value: nil}, nil
	}
	for _, list := range files {
		// 未显式指定名字时，按 map 的第一次非空文件作为默认值返回。
		// 这是“兜底可用”策略，不保证稳定顺序，因此更推荐显式绑定字段名。
		if len(list) > 0 {
			return File{Value: list[0]}, nil
		}
	}
	return File{Value: nil}, nil
}

func resolveFiles(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	filesAny, _ := ctx.Get(MetadataKeyFiles)
	files, _ := filesAny.(map[string][]*UploadedFile)
	if len(files) == 0 {
		return Files{Value: nil}, nil
	}
	if desc.BindingName != "" {
		return Files{Value: files[desc.BindingName]}, nil
	}
	out := make([]*UploadedFile, 0)
	for _, list := range files {
		// Files 不指定名字时聚合所有上传项，交给业务自行再分组。
		out = append(out, list...)
	}
	return Files{Value: out}, nil
}

func resolveCookie(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	m := cookiesSnapshot(ctx)
	if desc.BindingName == "" {
		value, err := decodeTo(desc.InnerType, m)
		if err != nil {
			return nil, err
		}
		return wrapValue(desc.WrapperType, value), nil
	}

	value, err := decodeTo(desc.InnerType, m[desc.BindingName])
	if err != nil {
		return nil, err
	}
	return wrapValue(desc.WrapperType, value), nil
}

func resolveCookies(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	m := cookiesSnapshot(ctx)
	out := make(map[string]string, len(m))
	maps.Copy(out, m)
	return Cookies(out), nil
}

func cookiesSnapshot(ctx *runtime.HandlerContext) map[string]string {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Get(MetadataKeyCookies); ok {
		if m, ok := v.(map[string]string); ok && m != nil {
			return m
		}
	}
	out := make(map[string]string)
	if req, ok := ctx.Request.Raw.(*http.Request); ok && req != nil {
		for _, c := range req.Cookies() {
			if c != nil {
				out[c.Name] = c.Value
			}
		}
	}
	// cookie 读取成本不高，但这里仍做一次请求内缓存，避免多个 resolver 重复遍历。
	ctx.Set(MetadataKeyCookies, out)
	return out
}
