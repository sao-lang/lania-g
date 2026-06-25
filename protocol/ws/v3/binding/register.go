// register.go 注册 WS 协议的默认 binding 声明与 compat 入口。
package ws

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	coreregistry "github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	wsprotocol "github.com/sao-lang/lania-g/protocol/ws/v3/protocol"
)

// metadata key 由 ws adapter 在连接建立/消息分发时写入，
// binding/ws 在参数解析阶段再读取这些值。
const (
	MetadataKeySocket  = "ws.socket"
	MetadataKeyServer  = "ws.server"
	MetadataKeyEvent   = "ws.event"
	MetadataKeyHeaders = "ws.headers"
	MetadataKeyContext = "ws.context"
	MetadataKeyValidator = "ws.validator"
)

// RegisterDefaults 将内置的 WS 参数绑定规则注册到 runtime。
func RegisterDefaults(rt *runtime.Runtime) {
	for _, reg := range DefaultRegistrations() {
		rt.RegisterBinding(runtime.NewBindingResolver(reg))
	}
}

// RegisterDefaultsToRegistry 将内置的 WS 参数绑定规则注册到 registry。
// 如果 reg 为空，则回退到全局 registry。
func RegisterDefaultsToRegistry(reg *coreregistry.Registry) {
	if reg == nil {
		RegisterDefaultsCompat()
		return
	}
	registerDefaultsToRegistry(reg)
}

// RegisterDefaultsCompat 显式保留“写入全局 registry”的兼容 binding 注册入口。
func RegisterDefaultsCompat() {
	registerDefaultsToRegistry(coreregistry.GlobalWithUsage("binding/ws.RegisterDefaultsCompat"))
}

func registerDefaultsToRegistry(reg *coreregistry.Registry) {
	reg.RegisterBindings(DefaultResolvers()...)
}

// DefaultRegistrations 返回 WS 协议默认启用的一组 binding registration。
//
// 其中：
// - `WSMessageBody[T]` 负责消息体解码
// - `Header[T]` / `Headers` 负责握手头或消息头注入
// - `AutoStruct` / `CompositeStruct` 是最后的 struct 兜底
func DefaultRegistrations() []runtime.BindingRegistration {
	allowed := map[runtime.Protocol]bool{wsprotocol.Protocol: true}
	return []runtime.BindingRegistration{
		registration("HandlerContext", nil, matchHandlerContext, resolveHandlerContext),
		registration("WsContext", allowed, matchWsContext, resolveWsContext),
		registration("WSMessageBody", allowed, matchGenericWrapper("WSMessageBody"), resolveMessageBody),
		registration("WSConnectedSocket", allowed, matchNamedType[WSConnectedSocket]("WSConnectedSocket"), resolveConnectedSocket),
		registration("WSEvent", allowed, matchNamedType[WSEvent]("WSEvent"), resolveEvent),
		registration("WSSocketID", allowed, matchNamedType[WSSocketID]("WSSocketID"), resolveSocketID),
		registration("WSRooms", allowed, matchNamedType[WSRooms]("WSRooms"), resolveRooms),
		registration("Header", allowed, matchGenericWrapper("Header"), resolveHeaders),
		registration("Headers", allowed, matchNamedType[Headers]("Headers"), resolveHeadersMap),
		registration("AutoStruct", allowed, matchAutoStruct, resolveAutoStruct),
		registration("CompositeStruct", allowed, matchCompositeStruct, resolveCompositeStruct),
	}
}

// DefaultResolvers 返回 WS 协议默认启用的一组 binding resolver。
func DefaultResolvers() []runtime.BindingResolver {
	return runtime.NewBindingResolvers(DefaultRegistrations()...)
}

// registration 只是把 matcher/resolve 配对打包，避免重复字面量。
func registration(name string, allowed map[runtime.Protocol]bool, match func(reflect.Type) (runtime.WrapperDescriptor, bool), resolve func(*runtime.HandlerContext, runtime.WrapperDescriptor) (any, error)) runtime.BindingRegistration {
	return runtime.BindingRegistration{
		Name:             name,
		AllowedProtocols: allowed,
		Match:            match,
		Resolve:          resolve,
	}
}

func matchHandlerContext(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	ctxPtr := reflect.TypeFor[*runtime.HandlerContext]()
	if t == ctxPtr {
		return runtime.WrapperDescriptor{Kind: "HandlerContext", WrapperType: t, InnerType: t}, true
	}
	return runtime.WrapperDescriptor{}, false
}

func resolveHandlerContext(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	return ctx, nil
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
		original := t
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		// 只接受 binding/ws 自己定义的 wrapper，避免误匹配业务 struct。
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

func trimGenericName(name string) string {
	if idx := strings.Index(name, "["); idx >= 0 {
		return name[:idx]
	}
	return name
}

// PackagePath returns the runtime import path for this binding package.
func PackagePath() string {
	return reflect.TypeOf(WSEvent{}).PkgPath()
}

func resolveMessageBody(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	raw := any(ctx.Request.Body)
	if len(ctx.Request.BodyBytes) > 0 {
		// adapter 若已缓存原始消息字节，这里优先用字节切片做解码输入。
		raw = ctx.Request.BodyBytes
	}
	if desc.BindingName != "" {
		// 带 bindingName 时把消息体当对象，从中抽单个字段。
		if v, ok := messageField(ctx, desc.BindingName); ok {
			raw = v
		}
	}
	value, err := decodeTo(desc.InnerType, raw)
	if err != nil {
		return nil, err
	}
	return wrapValue(desc.WrapperType, value), nil
}

func resolveConnectedSocket(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	var v any
	if raw, ok := ctx.Get(MetadataKeySocket); ok {
		v = raw
	}
	out := WSConnectedSocket{Value: v}
	return out, nil
}

func resolveEvent(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if raw, ok := ctx.Get(MetadataKeyEvent); ok {
		if s, ok := raw.(string); ok {
			return WSEvent{Value: s}, nil
		}
	}
	return WSEvent{Value: ctx.Request.Method}, nil
}

func resolveSocketID(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if sock, ok := ctx.Get(MetadataKeySocket); ok {
		if ider, ok := sock.(interface{ ID() string }); ok {
			return WSSocketID{Value: ider.ID()}, nil
		}
	}
	return WSSocketID{Value: ""}, nil
}

func resolveRooms(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if sock, ok := ctx.Get(MetadataKeySocket); ok {
		if r, ok := sock.(interface{ Rooms() []string }); ok {
			return WSRooms{Value: append([]string{}, r.Rooms()...)}, nil
		}
	}
	return WSRooms{Value: nil}, nil
}

func resolveHeaders(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	h := http.Header{}
	if raw, ok := ctx.Get(MetadataKeyHeaders); ok {
		if hh, ok := raw.(http.Header); ok {
			h = hh
		} else {
			value, err := decodeTo(desc.InnerType, raw)
			if err != nil {
				return nil, err
			}
			return wrapValue(desc.WrapperType, value), nil
		}
	} else {
		// metadata 未显式提供时，退化为基于 runtime.Request.HeadersMulti 构造 header 视图。
		for k, v := range ctx.Request.HeadersMulti {
			vv := append([]string{}, v...)
			h[k] = vv
		}
	}

	if desc.BindingName != "" {
		value, err := decodeTo(desc.InnerType, h.Get(desc.BindingName))
		if err != nil {
			return nil, err
		}
		return wrapValue(desc.WrapperType, value), nil
	}

	value, err := decodeTo(desc.InnerType, h)
	if err != nil {
		return nil, err
	}
	return wrapValue(desc.WrapperType, value), nil
}

func resolveHeadersMap(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	h := http.Header{}
	if raw, ok := ctx.Get(MetadataKeyHeaders); ok {
		if hh, ok := raw.(http.Header); ok {
			h = hh
		}
	}
	if len(h) == 0 {
		for k, v := range ctx.Request.HeadersMulti {
			h[k] = append([]string{}, v...)
		}
	}
	out := Headers{}
	for k, v := range h {
		out[k] = append([]string{}, v...)
	}
	return out, nil
}

func matchWsContext(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	ctxIface := reflect.TypeFor[Context]()
	wsCtxPtr := reflect.TypeFor[*WsContext]()
	if t == ctxIface || t == wsCtxPtr {
		return runtime.WrapperDescriptor{Kind: "WsContext", WrapperType: t, InnerType: t}, true
	}
	return runtime.WrapperDescriptor{}, false
}

func resolveWsContext(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if existing, ok := ctx.Get(MetadataKeyContext); ok {
		if wc, ok := existing.(*WsContext); ok && wc != nil {
			if desc.WrapperType.Kind() == reflect.Interface {
				return Context(wc), nil
			}
			return wc, nil
		}
	}
	// WsContext 只在单次消息处理内构造一次，避免多处参数注入重复包装。
	wc := NewWsContext(ctx)
	ctx.Set(MetadataKeyContext, wc)
	if desc.WrapperType.Kind() == reflect.Interface {
		return Context(wc), nil
	}
	return wc, nil
}

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

// resolveAutoStruct 用于“整条 WS 消息体直接映射到业务 struct”的兜底路径。
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

	if ptr {
		return outPtr.Interface(), nil
	}
	return outPtr.Elem().Interface(), nil
}

func decodeTo(target reflect.Type, raw any) (reflect.Value, error) {
	if target.Kind() == reflect.Interface {
		return reflect.ValueOf(raw), nil
	}
	if raw == nil {
		return zero(target), nil
	}

	rv := reflect.ValueOf(raw)
	if rv.IsValid() {
		if rv.Type().AssignableTo(target) {
			return rv, nil
		}
		if rv.Type().ConvertibleTo(target) {
			return rv.Convert(target), nil
		}
	}

	switch v := raw.(type) {
	case []byte:
		if target.Kind() == reflect.Slice && target.Elem().Kind() == reflect.Uint8 {
			return reflect.ValueOf(v), nil
		}
		if target.Kind() == reflect.String {
			return reflect.ValueOf(string(v)).Convert(target), nil
		}
		return decodeJSON(target, v)
	case string:
		if target.Kind() == reflect.String {
			return reflect.ValueOf(v).Convert(target), nil
		}
		if target.Kind() == reflect.Slice && target.Elem().Kind() == reflect.Uint8 {
			return reflect.ValueOf([]byte(v)).Convert(target), nil
		}
		return decodeJSON(target, []byte(v))
	default:
		// 对 map/struct 等对象先转成 JSON，再统一复用 JSON 解码逻辑。
		b, err := json.Marshal(v)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("encode ws message: %w", err)
		}
		return decodeJSON(target, b)
	}
}

func decodeJSON(target reflect.Type, data []byte) (reflect.Value, error) {
	if target.Kind() == reflect.Ptr {
		out := reflect.New(target.Elem())
		if len(data) > 0 {
			if err := json.Unmarshal(data, out.Interface()); err != nil {
				return reflect.Value{}, err
			}
		}
		return out, nil
	}

	out := reflect.New(target)
	if len(data) > 0 {
		if err := json.Unmarshal(data, out.Interface()); err != nil {
			return reflect.Value{}, err
		}
	}
	return out.Elem(), nil
}

func wrapValue(wrapperType reflect.Type, value reflect.Value) any {
	if wrapperType.Kind() == reflect.Ptr {
		wrapperType = wrapperType.Elem()
	}
	wrapper := reflect.New(wrapperType).Elem()
	field := wrapper.FieldByName("Value")
	if field.IsValid() && field.CanSet() && value.IsValid() {
		if value.Type().AssignableTo(field.Type()) {
			field.Set(value)
		} else if value.Type().ConvertibleTo(field.Type()) {
			field.Set(value.Convert(field.Type()))
		}
	}
	return wrapper.Interface()
}

func zero(t reflect.Type) reflect.Value {
	if t.Kind() == reflect.Ptr {
		return reflect.Zero(t)
	}
	return reflect.New(t).Elem()
}
