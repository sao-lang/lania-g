package grpc

import (
	stdctx "context"
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	grpcbinding "github.com/sao-lang/lania-g/protocol/grpc/v3/binding"
	"google.golang.org/protobuf/proto"
)

// validateMethodDefinition 在编译阶段前置约束每种 RPC 模式允许出现的参数/返回值形态，
// 尽量把 streaming 签名错误拦在启动前，而不是等运行期 binding 失败。
func validateMethodDefinition(def *MethodDefinition, handler *runtime.Handler) error {
	if def == nil || handler == nil || handler.Meta == nil {
		return fmt.Errorf("invalid grpc method declaration")
	}
	mode := def.Mode.Normalize()
	streams := scanSignature(handler.Meta.ParamTypes)

	switch mode {
	case RPCModeUnary:
		// unary 仍然沿用“单请求 + 单响应”的原有模型，因此任何 stream wrapper 都视为配置错误。
		if streams.raw > 0 || streams.server > 0 || streams.client > 0 || streams.bidi > 0 {
			return fmt.Errorf("grpc unary method %s/%s cannot declare stream wrappers", def.Service, def.Method)
		}
	case RPCModeServerStream:
		// server-streaming 允许一个首请求消息 + 一个发送流 wrapper，
		// 这样既能复用现有请求绑定，也能把后续多次 Send 显式交给业务控制。
		if streams.server != 1 {
			return fmt.Errorf("grpc server-stream method %s/%s requires exactly one grpcbinding.ServerStream[...] parameter or CompositeStruct field", def.Service, def.Method)
		}
		if streams.client > 0 || streams.bidi > 0 {
			return fmt.Errorf("grpc server-stream method %s/%s cannot mix client/bidi stream wrappers", def.Service, def.Method)
		}
		if streams.request > 1 {
			return fmt.Errorf("grpc server-stream method %s/%s can declare at most one request message parameter", def.Service, def.Method)
		}
		if err := validateStreamLikeReturns(def, handler.Meta.ReturnTypes); err != nil {
			return err
		}
	case RPCModeClientStream:
		// client-streaming 不再存在唯一请求体，业务必须显式从 stream 中逐条 Recv。
		if streams.client != 1 {
			return fmt.Errorf("grpc client-stream method %s/%s requires exactly one grpcbinding.ClientStream[...] parameter or CompositeStruct field", def.Service, def.Method)
		}
		if streams.server > 0 || streams.bidi > 0 {
			return fmt.Errorf("grpc client-stream method %s/%s cannot mix server/bidi stream wrappers", def.Service, def.Method)
		}
		if streams.request > 0 {
			return fmt.Errorf("grpc client-stream method %s/%s cannot declare Req[T] or request message parameters", def.Service, def.Method)
		}
	case RPCModeBidiStream:
		// bidi-streaming 的核心约束是“同一个 wrapper 同时承担 Recv/Send”，
		// 避免把双向流拆成两个不相关的参数后造成语义混乱。
		if streams.bidi != 1 {
			return fmt.Errorf("grpc bidi-stream method %s/%s requires exactly one grpcbinding.BidiStream[..., ...] parameter or CompositeStruct field", def.Service, def.Method)
		}
		if streams.server > 0 || streams.client > 0 {
			return fmt.Errorf("grpc bidi-stream method %s/%s cannot mix separate server/client stream wrappers", def.Service, def.Method)
		}
		if streams.request > 0 {
			return fmt.Errorf("grpc bidi-stream method %s/%s cannot declare Req[T] or request message parameters", def.Service, def.Method)
		}
		if err := validateStreamLikeReturns(def, handler.Meta.ReturnTypes); err != nil {
			return err
		}
	default:
		return fmt.Errorf("grpc method %s/%s uses unsupported mode %q", def.Service, def.Method, mode)
	}

	return nil
}

type signatureScan struct {
	request int
	raw     int
	server  int
	client  int
	bidi    int
}

// scanSignature 先把参数按“请求消息 / stream wrapper / raw stream”粗分类，
// 这样模式校验逻辑可以只关心数量和组合，而不必重复做反射判断。
func scanSignature(paramTypes []reflect.Type) signatureScan {
	var out signatureScan
	for _, paramType := range paramTypes {
		out.add(scanGRPCSignatureType(paramType))
	}
	return out
}

func (s *signatureScan) add(other signatureScan) {
	s.request += other.request
	s.raw += other.raw
	s.server += other.server
	s.client += other.client
	s.bidi += other.bidi
}

func scanGRPCSignatureType(t reflect.Type) signatureScan {
	var out signatureScan
	switch classifyGRPCParam(t) {
	case "request":
		out.request++
		return out
	case "raw_stream":
		out.raw++
		return out
	case "server_stream":
		out.server++
		return out
	case "client_stream":
		out.client++
		return out
	case "bidi_stream":
		out.bidi++
		return out
	}
	fields, ok := compositeFields(t)
	if !ok {
		return out
	}
	for _, field := range fields {
		switch classifyCompositeField(field) {
		case "request":
			out.request++
		case "raw_stream":
			out.raw++
		case "server_stream":
			out.server++
		case "client_stream":
			out.client++
		case "bidi_stream":
			out.bidi++
		}
	}
	return out
}

// classifyGRPCParam 只识别 gRPC adapter 自己关心的几类参数，
// 其余参数留给通用 DI/binding 流程处理。
func classifyGRPCParam(t reflect.Type) string {
	if t == nil || t == reflect.TypeFor[stdctx.Context]() {
		return ""
	}
	if bindingName := grpcBindingName(t); bindingName != "" {
		switch bindingName {
		case "Req":
			return "request"
		case "RawServerStream":
			return "raw_stream"
		case "ServerStream":
			return "server_stream"
		case "ClientStream":
			return "client_stream"
		case "BidiStream":
			return "bidi_stream"
		default:
			return ""
		}
	}
	if isProtoMessageType(t) {
		return "request"
	}
	return ""
}

func grpcBindingName(t reflect.Type) string {
	if t == nil {
		return ""
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.PkgPath() != grpcbinding.PackagePath() {
		return ""
	}
	return trimBindingGenericName(t.Name())
}

func trimBindingGenericName(name string) string {
	if idx := indexGenericName(name); idx >= 0 {
		return name[:idx]
	}
	return name
}

func indexGenericName(name string) int {
	for i := 0; i < len(name); i++ {
		if name[i] == '[' {
			return i
		}
	}
	return -1
}

func isProtoMessageType(t reflect.Type) bool {
	protoMsg := reflect.TypeFor[proto.Message]()
	return t != nil && t.Kind() == reflect.Ptr && t.Implements(protoMsg)
}

func validateStreamLikeReturns(def *MethodDefinition, returnTypes []reflect.Type) error {
	if len(returnTypes) == 0 {
		return nil
	}
	if len(returnTypes) == 1 && returnTypes[0] == reflect.TypeFor[error]() {
		return nil
	}
	return fmt.Errorf("grpc stream method %s/%s should return only error or no value", def.Service, def.Method)
}

// inferMethodRequestType 只为 unary / server-streaming 推断首个请求消息；
// client-stream 和 bidi-stream 没有“唯一请求体”，因此这里直接返回 nil。
func inferMethodRequestType(def *MethodDefinition) (reflect.Type, error) {
	if def == nil || def.Receiver == nil || def.HandlerName == "" {
		return nil, fmt.Errorf("invalid grpc method definition")
	}
	switch def.Mode.Normalize() {
	case RPCModeClientStream, RPCModeBidiStream:
		return nil, nil
	}
	if def.RequestType != nil {
		rt := def.RequestType
		if rt.Kind() != reflect.Ptr {
			rt = reflect.PointerTo(rt)
		}
		return rt, nil
	}
	recv := reflect.TypeOf(def.Receiver)
	if recv.Kind() != reflect.Ptr {
		recv = reflect.PointerTo(recv)
	}
	m, ok := recv.MethodByName(def.HandlerName)
	if !ok {
		return nil, fmt.Errorf("grpc receiver %s has no method %s", recv.String(), def.HandlerName)
	}
	mt := m.Type
	for i := 1; i < mt.NumIn(); i++ {
		pt := mt.In(i)
		if classifyGRPCParam(pt) == "request" {
			if bindingName := grpcBindingName(pt); bindingName == "Req" {
				if inner, ok := unwrapReqWrapper(pt); ok {
					return inner, nil
				}
				continue
			}
			if isProtoMessageType(pt) {
				return pt, nil
			}
		}
		if reqType, ok := firstCompositeRequestType(pt); ok {
			if reqType.Kind() != reflect.Ptr {
				reqType = reflect.PointerTo(reqType)
			}
			return reqType, nil
		}
	}
	return nil, fmt.Errorf("cannot infer grpc request type for %s/%s in %s mode", def.Service, def.Method, def.Mode.Normalize())
}

func compositeFields(t reflect.Type) ([]reflect.StructField, bool) {
	if t == nil {
		return nil, false
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || t.PkgPath() == grpcbinding.PackagePath() {
		return nil, false
	}
	fields := make([]reflect.StructField, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		if classifyCompositeField(field) != "" {
			fields = append(fields, field)
		}
	}
	if len(fields) == 0 {
		return nil, false
	}
	return fields, true
}

func classifyCompositeField(field reflect.StructField) string {
	switch {
	case field.Type == reflect.TypeFor[*runtime.HandlerContext](),
		field.Type == reflect.TypeFor[stdctx.Context](),
		field.Type == reflect.TypeFor[grpcbinding.GRPCContext](),
		field.Type == reflect.TypeOf(grpcbinding.Metadata(nil)),
		field.Type == reflect.TypeOf(grpcbinding.FullMethod("")),
		field.Type == reflect.TypeOf(grpcbinding.Service("")),
		field.Type == reflect.TypeOf(grpcbinding.Method("")):
		return "contextual"
	}
	if bindingName := grpcBindingName(field.Type); bindingName != "" {
		switch bindingName {
		case "Req":
			return "request"
		case "RawServerStream":
			return "raw_stream"
		case "ServerStream":
			return "server_stream"
		case "ClientStream":
			return "client_stream"
		case "BidiStream":
			return "bidi_stream"
		case "Header":
			return "header"
		default:
			return ""
		}
	}
	if field.Tag.Get(grpcbinding.TagReq) != "" {
		return "request"
	}
	if field.Tag.Get(grpcbinding.TagHeader) != "" {
		return "header"
	}
	return ""
}

func firstCompositeRequestType(t reflect.Type) (reflect.Type, bool) {
	fields, ok := compositeFields(t)
	if !ok {
		return nil, false
	}
	for _, field := range fields {
		if classifyCompositeField(field) != "request" {
			continue
		}
		if bindingName := grpcBindingName(field.Type); bindingName == "Req" {
			if inner, ok := unwrapReqWrapper(field.Type); ok {
				return inner, true
			}
			continue
		}
		return field.Type, true
	}
	return nil, false
}
