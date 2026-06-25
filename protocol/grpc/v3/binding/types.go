// types.go 定义 gRPC 协议暴露给 handler 的 binding wrapper 与辅助类型。
package grpc

import (
	stdctx "context"
	"fmt"
	"reflect"

	gogrpc "google.golang.org/grpc"
)

// Req 用于显式声明“参数从 gRPC 请求消息取值”。
// 它更偏 binding 风格：业务显式说“这里依赖请求消息”，而不是直接把 proto message 写进签名。
type Req[T any] struct{ Value T }

// Header 用于显式声明“参数从 gRPC incoming metadata 中取值”。
// 具体读哪个 metadata key，由 DSL 记录到 ParamBindings 后再在运行期解析。
type Header[T any] struct{ Value T }

// Metadata 是对 gRPC incoming metadata 的简化视图。
// 它表示为 `key -> []value` 的多值映射。
type Metadata map[string][]string

// Headers 是 Metadata 的别名，主要出于兼容性与可读性考虑。
type Headers = Metadata

// Context 表示标准库的 `context.Context`，可通过 binding 注入到 handler。
// 这里直接做类型别名，保证业务代码仍能无缝调用原生 `context.Context` API。
type Context = stdctx.Context

// FullMethod 表示 gRPC 的完整方法名，例如 `"/<Service>/<Method>"`。
type FullMethod string

// Service 表示在 DSL 中声明的 service 名称，通常对应 proto service 名，不带前导 `/`。
type Service string

// Method 表示在 DSL 中声明的 RPC 方法名。
type Method string

// RawServerStream 暴露最原始的 gRPC server stream。
// 高级场景下可直接访问底层 API。
type RawServerStream struct {
	gogrpc.ServerStream
}

// ServerStream 表示服务端流式发送能力。
type ServerStream[T any] struct {
	Raw gogrpc.ServerStream
}

func (s ServerStream[T]) Send(msg T) error {
	// ServerStream 只暴露发送侧能力，避免把“只写响应流”的场景建模得过重。
	return s.Raw.SendMsg(msg)
}

// ClientStream 表示客户端流式接收能力。
type ClientStream[T any] struct {
	Raw gogrpc.ServerStream
}

func (s ClientStream[T]) Recv() (T, error) {
	// 每次 Recv 都即时从底层 stream 读取一条消息，不在 wrapper 内做缓冲。
	return recvStreamMessage[T](s.Raw)
}

// BidiStream 表示双向流收发能力。
type BidiStream[Req any, Resp any] struct {
	Raw gogrpc.ServerStream
}

func (s BidiStream[Req, Resp]) Recv() (Req, error) {
	return recvStreamMessage[Req](s.Raw)
}

func (s BidiStream[Req, Resp]) Send(msg Resp) error {
	return s.Raw.SendMsg(msg)
}

func recvStreamMessage[T any](stream gogrpc.ServerStream) (T, error) {
	var zero T
	// 先按 T 的运行时类型创建一个可交给 grpc 反序列化器直接填充的目标对象。
	target, err := newStreamMessageTarget(reflect.TypeFor[T]())
	if err != nil {
		return zero, err
	}
	if err := stream.RecvMsg(target); err != nil {
		return zero, err
	}
	value := reflect.ValueOf(target)
	want := reflect.TypeFor[T]()
	// 这里同时兼容 `T` 是指针消息和结构体值两种写法，避免业务因为泛型实例化方式不同而失败。
	if value.IsValid() && value.Type().AssignableTo(want) {
		return value.Interface().(T), nil
	}
	if value.IsValid() && value.Kind() == reflect.Ptr && value.Elem().IsValid() && value.Elem().Type().AssignableTo(want) {
		return value.Elem().Interface().(T), nil
	}
	if value.IsValid() && value.Type().ConvertibleTo(want) {
		return value.Convert(want).Interface().(T), nil
	}
	if value.IsValid() && value.Kind() == reflect.Ptr && value.Elem().IsValid() && value.Elem().Type().ConvertibleTo(want) {
		return value.Elem().Convert(want).Interface().(T), nil
	}
	return zero, fmt.Errorf("grpc stream received %T, cannot assign to %s", target, want.String())
}

func newStreamMessageTarget(t reflect.Type) (any, error) {
	if t == nil {
		return nil, fmt.Errorf("nil grpc stream message type")
	}
	if t.Kind() == reflect.Ptr {
		return reflect.New(t.Elem()).Interface(), nil
	}
	if t.Kind() == reflect.Struct {
		return reflect.New(t).Interface(), nil
	}
	return nil, fmt.Errorf("unsupported grpc stream message type: %s", t.String())
}
