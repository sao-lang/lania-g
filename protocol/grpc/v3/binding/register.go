// register.go 注册 gRPC 协议的默认 binding 声明与 compat 入口。
package grpc

import (
	stdctx "context"
	"fmt"
	"reflect"
	"strings"

	gogrpc "google.golang.org/grpc"
	grpcmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	coreregistry "github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	grpcprotocol "github.com/sao-lang/lania-g/protocol/grpc/v3/protocol"
)

// 这组 metadata key 由 grpc adapter 在请求入口写入 HandlerContext.Metadata，
// binding/grpc 只按约定读取，不直接依赖 transport 细节。
const (
	MetadataKeyIncomingMetadata = "grpc.incoming.metadata"
	MetadataKeyFullMethod       = "grpc.full_method"
	MetadataKeyService          = "grpc.service"
	MetadataKeyMethod           = "grpc.method"
	MetadataKeyMode             = "grpc.mode"
)

// RegisterDefaults 将内置的 gRPC 参数绑定规则注册到 runtime。
func RegisterDefaults(rt *runtime.Runtime) {
	for _, reg := range DefaultRegistrations() {
		rt.RegisterBinding(runtime.NewBindingResolver(reg))
	}
}

// RegisterDefaultsToRegistry 将内置的 gRPC 参数绑定规则注册到 registry。
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
	registerDefaultsToRegistry(coreregistry.GlobalWithUsage("binding/grpc.RegisterDefaultsCompat"))
}

func registerDefaultsToRegistry(reg *coreregistry.Registry) {
	reg.RegisterBindings(DefaultResolvers()...)
}

// DefaultRegistrations 返回 gRPC 协议默认启用的一组 binding registration。
//
// gRPC 这里的核心区别是：
// - 请求消息通常已经是强类型 proto message，不强调 HTTP/WS 那种多来源拼装
// - metadata/header 语义严格区分单值 Header[T] 与整包 Metadata/Headers
func DefaultRegistrations() []runtime.BindingRegistration {
	allowed := map[runtime.Protocol]bool{grpcprotocol.Protocol: true}
	return []runtime.BindingRegistration{
		registration("HandlerContext", nil, matchHandlerContext, resolveHandlerContext),
		registration("GRPCContext", allowed, matchGRPCContext, resolveGRPCContext),
		registration("Context", allowed, matchStdContext, resolveStdContext),
		registration("Req", allowed, matchGenericWrapper("Req"), resolveReqWrapper),
		registration("RequestMessage", allowed, matchRequestMessage, resolveRequestMessage),
		registration("Metadata", allowed, matchNamedType[Metadata]("Metadata"), resolveIncomingMetadata),
		registration("Headers", allowed, matchNamedType[Headers]("Headers"), resolveIncomingMetadata),
		registration("Header", allowed, matchGenericWrapper("Header"), resolveHeader),
		registration("FullMethod", allowed, matchNamedType[FullMethod]("FullMethod"), resolveFullMethod),
		registration("Service", allowed, matchNamedType[Service]("Service"), resolveService),
		registration("Method", allowed, matchNamedType[Method]("Method"), resolveMethod),
		registration("RawServerStream", allowed, matchRawServerStream, resolveRawServerStream),
		registration("ServerStream", allowed, matchServerStream, resolveServerStream),
		registration("ClientStream", allowed, matchClientStream, resolveClientStream),
		registration("BidiStream", allowed, matchBidiStream, resolveBidiStream),
		registration("CompositeStruct", allowed, matchCompositeStruct, resolveCompositeStruct),
	}
}

// DefaultResolvers 返回 gRPC 协议默认启用的一组 binding resolver。
func DefaultResolvers() []runtime.BindingResolver {
	return runtime.NewBindingResolvers(DefaultRegistrations()...)
}

// registration 只是局部薄封装，用来减少重复字面量。
func registration(name string, allowed map[runtime.Protocol]bool, match func(reflect.Type) (runtime.WrapperDescriptor, bool), resolve func(*runtime.HandlerContext, runtime.WrapperDescriptor) (any, error)) runtime.BindingRegistration {
	return runtime.BindingRegistration{
		Name:             name,
		AllowedProtocols: allowed,
		Match:            match,
		Resolve:          resolve,
	}
}

// matchHandlerContext 允许 handler 直接拿到底层 runtime.HandlerContext。
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

// matchStdContext 匹配标准库 `context.Context`。
// 这样业务方法既能走框架 binding，也能继续复用 gRPC 生态普遍依赖的上下文接口。
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

// matchGenericWrapper 负责识别 `Req[T]`、`Header[T]` 这类“带 Value 字段的泛型 struct wrapper”。
// 这里故意要求：
// 1. 类型必须来自当前 binding 包
// 2. 结构中必须有 `Value` 字段
// 以避免误吞掉业务自己的泛型 DTO。
func matchGenericWrapper(baseName string) func(reflect.Type) (runtime.WrapperDescriptor, bool) {
	return func(t reflect.Type) (runtime.WrapperDescriptor, bool) {
		original := t
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		// 只匹配 binding/grpc 自己定义的 wrapper，避免把业务 DTO 吞进来。
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

func matchRawServerStream(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	base := reflect.TypeOf(RawServerStream{})
	if t != base {
		return runtime.WrapperDescriptor{}, false
	}
	return runtime.WrapperDescriptor{Kind: "RawServerStream", WrapperType: t, InnerType: reflect.TypeFor[gogrpc.ServerStream]()}, true
}

func matchServerStream(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	return matchStreamWrapper(t, "ServerStream", inspectServerStreamType)
}

func matchClientStream(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	return matchStreamWrapper(t, "ClientStream", inspectClientStreamType)
}

func matchBidiStream(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	return matchStreamWrapper(t, "BidiStream", inspectBidiStreamType)
}

// matchStreamWrapper 识别新的 streaming wrapper。
// 它不依赖具体的泛型实例名，而是通过：
// - 包路径
// - 基础类型名
// - `Raw grpc.ServerStream` 字段
// - Send/Recv 方法签名
// 共同确认这是一个合法的 gRPC stream wrapper。
func matchStreamWrapper(t reflect.Type, baseName string, inspect func(reflect.Type) (reflect.Type, bool)) (runtime.WrapperDescriptor, bool) {
	if t == nil {
		return runtime.WrapperDescriptor{}, false
	}
	original := t
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return runtime.WrapperDescriptor{}, false
	}
	if t.PkgPath() != PackagePath() || trimGenericName(t.Name()) != baseName {
		return runtime.WrapperDescriptor{}, false
	}
	if field, ok := t.FieldByName("Raw"); !ok || field.Type != reflect.TypeFor[gogrpc.ServerStream]() {
		return runtime.WrapperDescriptor{}, false
	}
	inner, ok := inspect(t)
	if !ok {
		return runtime.WrapperDescriptor{}, false
	}
	return runtime.WrapperDescriptor{Kind: baseName, WrapperType: original, InnerType: inner}, true
}

func inspectServerStreamType(t reflect.Type) (reflect.Type, bool) {
	// 方法接收者会占掉第一个入参，因此这里要求 NumIn()==2，
	// 第二个参数才是业务真正发送的消息类型。
	send, ok := t.MethodByName("Send")
	if !ok || send.Type.NumIn() != 2 || send.Type.NumOut() != 1 || send.Type.Out(0) != reflect.TypeFor[error]() {
		return nil, false
	}
	return send.Type.In(1), true
}

func inspectClientStreamType(t reflect.Type) (reflect.Type, bool) {
	// Recv 只应该返回 `(T, error)`，不接受额外输入参数；
	// 如果签名不满足这个形态，就不把它认作 client-stream wrapper。
	recv, ok := t.MethodByName("Recv")
	if !ok || recv.Type.NumIn() != 1 || recv.Type.NumOut() != 2 || recv.Type.Out(1) != reflect.TypeFor[error]() {
		return nil, false
	}
	return recv.Type.Out(0), true
}

func inspectBidiStreamType(t reflect.Type) (reflect.Type, bool) {
	// bidi wrapper 必须同时满足 client-stream 和 server-stream 两侧能力，
	// 否则会在这里被排除。
	recvType, ok := inspectClientStreamType(t)
	if !ok {
		return nil, false
	}
	if _, ok := inspectServerStreamType(t); !ok {
		return nil, false
	}
	return recvType, true
}

// trimGenericName 把 `Req[*pb.Foo]` 这类实例化名字裁成 `Req`，
// 用于 wrapper 识别。
func trimGenericName(name string) string {
	if idx := strings.Index(name, "["); idx >= 0 {
		return name[:idx]
	}
	return name
}

// PackagePath 返回当前 binding 包在运行时的导入路径。
func PackagePath() string {
	return reflect.TypeOf(FullMethod("")).PkgPath()
}

// resolveReqWrapper 处理 `Req[T]`。
// 语义上它不是再去“重新解码请求”，而是把 transport 已经解好的 request message 投影成 T。
func resolveReqWrapper(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if err := ensureModeAllowed(ctx, "Req", "unary", "server_stream"); err != nil {
		return nil, err
	}
	raw := any(nil)
	if ctx != nil && ctx.Request != nil {
		raw = ctx.Request.Body
	}
	// unary / server-streaming 的首请求消息都放在 Request.Body，
	// 因此 `Req[T]` 仍然可以沿用旧的取值方式。
	value, err := decodeTo(desc.InnerType, raw)
	if err != nil {
		return nil, err
	}
	return wrapValue(desc.WrapperType, value), nil
}

// matchRequestMessage 用于匹配直接声明的请求消息参数，例如 `*pb.FooRequest`。
// 这里刻意保持保守：只匹配 `google.golang.org/protobuf/proto.Message`，
// 避免把任意 `*struct` 参数（例如 `*Repo`）的 DI 注入吞掉。
func matchRequestMessage(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	original := t
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	// 避免命中本包自己的 wrapper 类型。
	if t.PkgPath() == PackagePath() {
		return runtime.WrapperDescriptor{}, false
	}
	// 大多数 proto message 都是“实现了 `proto.Message` 的结构体指针”。
	protoMsg := reflect.TypeFor[proto.Message]()
	if original != nil && original.Kind() == reflect.Ptr && t.Kind() == reflect.Struct && original.Implements(protoMsg) {
		return runtime.WrapperDescriptor{Kind: "RequestMessage", WrapperType: original, InnerType: original}, true
	}
	return runtime.WrapperDescriptor{}, false
}

// resolveRequestMessage 直接注入原始请求消息本身，例如 `*pb.FooRequest`。
// 它比 `Req[T]` 更保守：只有 transport 传进来的对象本身可赋值/可转换时才返回。
func resolveRequestMessage(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if err := ensureModeAllowed(ctx, "RequestMessage", "unary", "server_stream"); err != nil {
		return nil, err
	}
	raw := any(nil)
	if ctx != nil && ctx.Request != nil {
		raw = ctx.Request.Body
	}
	if raw == nil {
		return reflect.Zero(desc.WrapperType).Interface(), nil
	}
	// 这里不做宽松转换或重新解码，只在 transport 已经给出正确消息对象时才直接透传。
	rv := reflect.ValueOf(raw)
	if rv.IsValid() && rv.Type().AssignableTo(desc.WrapperType) {
		return raw, nil
	}
	if rv.IsValid() && rv.Type().ConvertibleTo(desc.WrapperType) {
		return rv.Convert(desc.WrapperType).Interface(), nil
	}
	return reflect.Zero(desc.WrapperType).Interface(), nil
}

// resolveIncomingMetadata 返回 incoming metadata 的快照副本，
// 避免业务层直接修改底层 metadata map。
func resolveIncomingMetadata(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	md := map[string][]string(incomingMetadata(ctx))
	out := make(map[string][]string, len(md))
	for k, v := range md {
		// 返回副本，避免业务层修改底层 metadata map。
		out[k] = append([]string{}, v...)
	}
	return Metadata(out), nil
}

// resolveHeader 处理 `Header[T]`。
// 它只负责单个 metadata key 的读取；整包 metadata 应改用 `Metadata` / `Headers`。
func resolveHeader(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if ctx == nil {
		return nil, fmt.Errorf("grpc Header[T] binding requires handler context")
	}
	md := incomingMetadata(ctx)

	// Header[T] 的设计目标是“单 key 单值”。
	// 不带 binding name 时含义不明确，因此只对少数“整包 map 类型”做兼容兜底。
	if desc.BindingName == "" {
		// 为了使用方便，允许 `Header[map[string][]string]` / `Header[grpc.Metadata]` 这类写法。
		if desc.InnerType == reflect.TypeOf(map[string][]string(nil)) {
			return wrapValue(desc.WrapperType, reflect.ValueOf(map[string][]string(md))), nil
		}
		if desc.InnerType == reflect.TypeOf(grpcmetadata.MD{}) {
			return wrapValue(desc.WrapperType, reflect.ValueOf(md)), nil
		}
		// `binding/grpc.Metadata` 是一个具名 map 类型。
		if desc.InnerType == reflect.TypeOf(Metadata(nil)) || desc.InnerType == reflect.TypeOf(Headers(nil)) {
			return wrapValue(desc.WrapperType, reflect.ValueOf(Metadata(map[string][]string(md)))), nil
		}
		return nil, fmt.Errorf("grpc Header[T] requires binding name (metadata key); add a `header:\"...\"` tag in CompositeStruct, use MethodBuilder.BindParam(...), or inject grpc.Metadata instead")
	}

	key := strings.ToLower(desc.BindingName)
	raw := ""
	if vals := md.Get(key); len(vals) > 0 {
		raw = vals[0]
	}
	value, err := decodeTo(desc.InnerType, raw)
	if err != nil {
		return nil, err
	}
	return wrapValue(desc.WrapperType, value), nil
}

func incomingMetadata(ctx *runtime.HandlerContext) grpcmetadata.MD {
	md := grpcmetadata.MD{}
	if ctx == nil {
		return md
	}
	if v, ok := ctx.Get(MetadataKeyIncomingMetadata); ok {
		switch vv := v.(type) {
		case grpcmetadata.MD:
			return vv
		case map[string][]string:
			return grpcmetadata.MD(vv)
		}
	}
	return md
}

func resolveRawServerStream(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if err := ensureModeAllowed(ctx, "RawServerStream", "server_stream", "client_stream", "bidi_stream"); err != nil {
		return nil, err
	}
	stream, err := getRawServerStream(ctx)
	if err != nil {
		return nil, err
	}
	return RawServerStream{ServerStream: stream}, nil
}

func resolveServerStream(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if err := ensureModeAllowed(ctx, "ServerStream", "server_stream"); err != nil {
		return nil, err
	}
	stream, err := getRawServerStream(ctx)
	if err != nil {
		return nil, err
	}
	// 这里不重新构造抽象层对象，只是把底层 grpc.ServerStream 填回 wrapper.Raw，
	// 由 wrapper 自己暴露 Send/Recv API，从而保持运行期零额外状态。
	return wrapStreamValue(desc.WrapperType, stream)
}

func resolveClientStream(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if err := ensureModeAllowed(ctx, "ClientStream", "client_stream"); err != nil {
		return nil, err
	}
	stream, err := getRawServerStream(ctx)
	if err != nil {
		return nil, err
	}
	// client-stream/bidi-stream 的消息读取由业务显式调用 Recv() 驱动，
	// binding 只负责把原始 stream 能力投影成强类型 wrapper。
	return wrapStreamValue(desc.WrapperType, stream)
}

func resolveBidiStream(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if err := ensureModeAllowed(ctx, "BidiStream", "bidi_stream"); err != nil {
		return nil, err
	}
	stream, err := getRawServerStream(ctx)
	if err != nil {
		return nil, err
	}
	return wrapStreamValue(desc.WrapperType, stream)
}

// resolveFullMethod / resolveService / resolveMethod 都只是把 transport 预写入的元信息
// 投影成更易读的命名类型，便于业务层显式声明依赖。
func resolveFullMethod(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if ctx != nil {
		if v, ok := ctx.Get(MetadataKeyFullMethod); ok {
			if s, ok2 := v.(string); ok2 {
				return FullMethod(s), nil
			}
			if fm, ok2 := v.(FullMethod); ok2 {
				return fm, nil
			}
		}
	}
	return FullMethod(""), nil
}

func resolveService(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if ctx != nil {
		if v, ok := ctx.Get(MetadataKeyService); ok {
			if s, ok2 := v.(string); ok2 {
				return Service(s), nil
			}
			if s, ok2 := v.(Service); ok2 {
				return s, nil
			}
		}
	}
	return Service(""), nil
}

func resolveMethod(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if ctx != nil {
		if v, ok := ctx.Get(MetadataKeyMethod); ok {
			if s, ok2 := v.(string); ok2 {
				return Method(s), nil
			}
			if s, ok2 := v.(Method); ok2 {
				return s, nil
			}
		}
	}
	// 兜底：如果可用，则从 route key 中拆出对应部分。
	if ctx != nil && ctx.RouteKey != "" {
		if rk, err := runtime.ParseRouteKey(ctx.RouteKey); err == nil {
			return Method(rk.Method), nil
		}
	}
	return Method(""), nil
}

func decodeTo(target reflect.Type, raw any) (reflect.Value, error) {
	if raw == nil {
		return zero(target), nil
	}
	rv := reflect.ValueOf(raw)
	if rv.IsValid() && rv.Type().AssignableTo(target) {
		return rv, nil
	}
	if rv.IsValid() && rv.Type().ConvertibleTo(target) {
		return rv.Convert(target), nil
	}
	// gRPC 请求通常已经是强类型对象，这里故意保持严格，不做 JSON 宽松兜底，
	// 避免把错误的 metadata/header 值静默吞掉。
	return reflect.Value{}, fmt.Errorf("cannot decode %T into %s", raw, target.String())
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

func getRawServerStream(ctx *runtime.HandlerContext) (gogrpc.ServerStream, error) {
	if ctx == nil || ctx.Request == nil || ctx.Request.Raw == nil {
		return nil, fmt.Errorf("grpc stream binding requires Request.Raw to carry grpc.ServerStream")
	}
	// streaming 场景下 transport 会把底层 stream 放进 Request.Raw；
	// 这里做一次集中校验，避免每个 resolver 重复写类型断言。
	stream, ok := ctx.Request.Raw.(gogrpc.ServerStream)
	if !ok || stream == nil {
		return nil, fmt.Errorf("grpc stream binding requires grpc.ServerStream, got %T", ctx.Request.Raw)
	}
	return stream, nil
}

// ensureModeAllowed 让 binding 本身也感知当前 RPC 模式，
// 即使业务签名绕过了编译期检查，也能在解析参数时给出更直接的错误。
func ensureModeAllowed(ctx *runtime.HandlerContext, binding string, allowed ...string) error {
	mode := currentMode(ctx)
	for _, item := range allowed {
		if mode == item {
			return nil
		}
	}
	return fmt.Errorf("grpc %s binding is not supported in %s mode", binding, mode)
}

func currentMode(ctx *runtime.HandlerContext) string {
	if ctx != nil {
		if value, ok := ctx.Get(MetadataKeyMode); ok {
			if mode, ok := value.(string); ok && mode != "" {
				return mode
			}
		}
	}
	// 兼容旧 unary 路径：如果 transport 没有显式写 mode，就默认按 unary 处理。
	return "unary"
}

func wrapStreamValue(wrapperType reflect.Type, stream gogrpc.ServerStream) (any, error) {
	original := wrapperType
	if wrapperType.Kind() == reflect.Ptr {
		wrapperType = wrapperType.Elem()
	}
	if wrapperType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("grpc stream wrapper must be a struct, got %s", original.String())
	}
	wrapper := reflect.New(wrapperType).Elem()
	field := wrapper.FieldByName("Raw")
	if !field.IsValid() || !field.CanSet() || field.Type() != reflect.TypeFor[gogrpc.ServerStream]() {
		return nil, fmt.Errorf("grpc stream wrapper %s must expose settable Raw grpc.ServerStream field", original.String())
	}
	// wrapper 只是一个轻量壳对象：真正的消息收发、header/trailer 能力仍由底层 stream 持有。
	field.Set(reflect.ValueOf(stream))
	if original.Kind() == reflect.Ptr {
		out := reflect.New(wrapperType)
		out.Elem().Set(wrapper)
		return out.Interface(), nil
	}
	return wrapper.Interface(), nil
}
