// register.go 注册 MQ 协议的默认 binding 声明与 compat 入口。
package mq

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	coreregistry "github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	mqprotocol "github.com/sao-lang/lania-g/protocol/mq/v3/protocol"
)

// metadata key 由 MQ adapter/driver 在投递消息时写入，
// binding/mq 只按约定读取，不自行推断驱动实现细节。
const (
	MetadataKeyHeaders    = "mq.headers"
	MetadataKeyTopic      = "mq.topic"
	MetadataKeyConsumer   = "mq.consumer"
	MetadataKeyKey        = "mq.key"
	MetadataKeyRetryCount = "mq.retry_count"
	MetadataKeyAck        = "mq.ack"
	MetadataKeyNack       = "mq.nack"
)

// RegisterDefaults 将内置的 MQ 参数绑定规则注册到 runtime。
func RegisterDefaults(rt *runtime.Runtime) {
	for _, reg := range DefaultRegistrations() {
		rt.RegisterBinding(runtime.NewBindingResolver(reg))
	}
}

// RegisterDefaultsToRegistry 将内置的 MQ 参数绑定规则注册到 registry。
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
	registerDefaultsToRegistry(coreregistry.GlobalWithUsage("binding/mq.RegisterDefaultsCompat"))
}

func registerDefaultsToRegistry(reg *coreregistry.Registry) {
	reg.RegisterBindings(DefaultResolvers()...)
}

// DefaultRegistrations 返回 MQ 协议默认启用的一组 binding registration。
//
// MQ binding 的重点在于：
// - `Message[T]` 负责消息体解码
// - `Header[T]` / `Headers` 负责消息头读取
// - `Ack` / `Nack` 把驱动层确认语义投影成 handler 可调用函数
func DefaultRegistrations() []runtime.BindingRegistration {
	allowed := map[runtime.Protocol]bool{mqprotocol.Protocol: true}
	return []runtime.BindingRegistration{
		registration("HandlerContext", nil, matchHandlerContext, resolveHandlerContext),
		registration("Context", allowed, matchStdContext, resolveStdContext),
		registration("Message", allowed, matchGenericWrapper("Message"), resolveMessage),
		registration("Header", allowed, matchGenericWrapper("Header"), resolveHeader),
		registration("Headers", allowed, matchNamedType[Headers]("Headers"), resolveHeaders),
		registration("Topic", allowed, matchNamedType[Topic]("Topic"), resolveTopic),
		registration("Consumer", allowed, matchNamedType[Consumer]("Consumer"), resolveConsumer),
		registration("Key", allowed, matchNamedType[Key]("Key"), resolveKey),
		registration("RetryCount", allowed, matchNamedType[RetryCount]("RetryCount"), resolveRetryCount),
		registration("Ack", allowed, matchNamedType[Ack]("Ack"), resolveAck),
		registration("Nack", allowed, matchNamedType[Nack]("Nack"), resolveNack),
		registration("AutoStruct", allowed, matchAutoStruct, resolveAutoStruct),
	}
}

// DefaultResolvers 返回 MQ 协议默认启用的一组 binding resolver。
func DefaultResolvers() []runtime.BindingResolver {
	return runtime.NewBindingResolvers(DefaultRegistrations()...)
}

// registration 是局部薄封装，避免重复写 BindingRegistration 字面量。
func registration(name string, allowed map[runtime.Protocol]bool, match func(reflect.Type) (runtime.WrapperDescriptor, bool), resolve func(*runtime.HandlerContext, runtime.WrapperDescriptor) (any, error)) runtime.BindingRegistration {
	return runtime.BindingRegistration{
		Name:             name,
		AllowedProtocols: allowed,
		Match:            match,
		Resolve:          resolve,
	}
}

// matchHandlerContext 允许 handler 直接注入底层 runtime.HandlerContext。
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

// matchStdContext 匹配标准库 `context.Context`，便于业务沿用通用超时/取消语义。
func matchStdContext(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	ctxIface := reflect.TypeFor[stdctx.Context]()
	if t == ctxIface {
		return runtime.WrapperDescriptor{Kind: "Context", WrapperType: t, InnerType: t}, true
	}
	return runtime.WrapperDescriptor{}, false
}

func resolveStdContext(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if ctx == nil {
		return stdctx.Background(), nil
	}
	return ctx.Context(), nil
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

// trimGenericName 把 `Message[*Foo]` 这类实例化名字裁成 `Message`，用于 wrapper 识别。
func trimGenericName(name string) string {
	if idx := strings.Index(name, "["); idx >= 0 {
		return name[:idx]
	}
	return name
}

// PackagePath returns the runtime import path for this binding package.
func PackagePath() string {
	return reflect.TypeOf(Topic("")).PkgPath()
}

// resolveMessage 处理 `Message[T]`。
// 它优先使用 `BodyBytes`，确保消息体若本来就是字节流时不会丢失原始编码。
func resolveMessage(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	raw := any(nil)
	if ctx != nil && ctx.Request != nil {
		if len(ctx.Request.BodyBytes) > 0 {
			raw = ctx.Request.BodyBytes
		} else {
			raw = ctx.Request.Body
		}
	}
	value, err := decodeTo(desc.InnerType, raw)
	if err != nil {
		return nil, err
	}
	return wrapValue(desc.WrapperType, value), nil
}

// resolveHeader 处理 `Header[T]`。
// MQ 里 header 语义是“按 key 取单值”；需要整包头时应改用 `Headers`。
func resolveHeader(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	headers := headersSnapshot(ctx)
	if desc.BindingName == "" {
		return nil, fmt.Errorf("mq Header[T] requires binding name; use MethodBuilder.BindParam(...) or inject mq.Headers instead")
	}
	raw := ""
	if list := headers[desc.BindingName]; len(list) > 0 {
		raw = list[0]
	}
	value, err := decodeTo(desc.InnerType, raw)
	if err != nil {
		return nil, err
	}
	return wrapValue(desc.WrapperType, value), nil
}

func resolveHeaders(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	// Headers 返回的是快照，保证后续业务改动不会回写到底层 driver metadata。
	return Headers(headersSnapshot(ctx)), nil
}

// 下面这组 resolver 都只是把 adapter 预写入的投递元信息投影成命名类型，
// 便于业务 handler 用显式参数拿到 topic/consumer/key/retry 等上下文。
func resolveTopic(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyTopic); ok {
		if s, ok2 := v.(string); ok2 {
			return Topic(s), nil
		}
	}
	return Topic(""), nil
}

func resolveConsumer(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyConsumer); ok {
		if s, ok2 := v.(string); ok2 {
			return Consumer(s), nil
		}
	}
	return Consumer(""), nil
}

func resolveKey(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyKey); ok {
		if s, ok2 := v.(string); ok2 {
			return Key(s), nil
		}
	}
	return Key(""), nil
}

func resolveRetryCount(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyRetryCount); ok {
		switch vv := v.(type) {
		case int:
			return RetryCount(vv), nil
		case RetryCount:
			return vv, nil
		}
	}
	return RetryCount(0), nil
}

func resolveAck(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyAck); ok {
		if fn, ok2 := v.(func() error); ok2 {
			return Ack(fn), nil
		}
		if fn, ok2 := v.(Ack); ok2 {
			return fn, nil
		}
	}
	// 未接入真实 ack 函数时返回 no-op，便于业务统一调用。
	return Ack(func() error { return nil }), nil
}

func resolveNack(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if v, ok := ctx.Get(MetadataKeyNack); ok {
		if fn, ok2 := v.(func(error) error); ok2 {
			return Nack(fn), nil
		}
		if fn, ok2 := v.(Nack); ok2 {
			return fn, nil
		}
	}
	// 同上，默认提供 no-op nack，避免每个 handler 都判空。
	return Nack(func(error) error { return nil }), nil
}

func matchAutoStruct(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	original := t
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || t.PkgPath() == PackagePath() {
		return runtime.WrapperDescriptor{}, false
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

// resolveAutoStruct 处理“整条消息体直接解到业务 struct”的兜底路径。
// 它是 MQ 最常见的使用姿势：把消息 payload 直接绑定到业务 DTO。
func resolveAutoStruct(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	t := desc.WrapperType
	ptr := false
	if t.Kind() == reflect.Ptr {
		ptr = true
		t = t.Elem()
	}
	outPtr := reflect.New(t)
	raw := any(ctx.Request.Body)
	if len(ctx.Request.BodyBytes) > 0 {
		raw = ctx.Request.BodyBytes
	}
	value, err := decodeTo(t, raw)
	if err != nil {
		return nil, err
	}
	outPtr.Elem().Set(value)
	if ptr {
		return outPtr.Interface(), nil
	}
	return outPtr.Elem().Interface(), nil
}

func headersSnapshot(ctx *runtime.HandlerContext) map[string][]string {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Get(MetadataKeyHeaders); ok {
		if m, ok2 := v.(map[string][]string); ok2 {
			out := make(map[string][]string, len(m))
			for k, vv := range m {
				// 始终拷贝切片，避免调用方修改共享 header 列表。
				out[k] = append([]string{}, vv...)
			}
			return out
		}
	}
	return map[string][]string{}
}

func decodeTo(target reflect.Type, raw any) (reflect.Value, error) {
	if raw == nil {
		return zero(target), nil
	}
	if b, ok := raw.([]byte); ok {
		return decodeJSON(target, b)
	}
	if s, ok := raw.(string); ok {
		return decodeString(target, s)
	}
	rv := reflect.ValueOf(raw)
	if rv.IsValid() && rv.Type().AssignableTo(target) {
		return rv, nil
	}
	if rv.IsValid() && rv.Type().ConvertibleTo(target) {
		return rv.Convert(target), nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return reflect.Value{}, err
	}
	return decodeJSON(target, data)
}

func decodeString(target reflect.Type, s string) (reflect.Value, error) {
	if strings.TrimSpace(s) == "" {
		return zero(target), nil
	}
	if target.Kind() == reflect.Ptr {
		v, err := decodeString(target.Elem(), s)
		if err != nil {
			return reflect.Value{}, err
		}
		ptr := reflect.New(target.Elem())
		ptr.Elem().Set(v)
		return ptr, nil
	}
	switch target.Kind() {
	case reflect.String:
		return reflect.ValueOf(s).Convert(target), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return reflect.Value{}, err
		}
		v := reflect.New(target).Elem()
		v.SetInt(i)
		return v, nil
	case reflect.Bool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return reflect.Value{}, err
		}
		v := reflect.New(target).Elem()
		v.SetBool(b)
		return v, nil
	default:
		return decodeJSON(target, []byte(strconv.Quote(s)))
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
