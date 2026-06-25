// context.go 定义 gRPC binding 使用的上下文适配层与请求绑定入口。
package grpc

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"

	"github.com/go-playground/validator/v10"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	MetadataKeyContext   = "grpc.context"
	MetadataKeyValidator = "grpc.validator"
)

// Validator 定义 gRPC DTO 的校验接口。
// `ShouldBindReq` / `ShouldBindStream` 在完成绑定后会尝试调用它。
type Validator interface {
	Validate(obj any) error
}

// GRPCContext 是 gRPC 协议暴露给 handler 的增强上下文抽象。
//
// 它兼容标准库 `context.Context`，同时额外提供：
// - metadata / method 信息读取
// - `ShouldBindReq`：把当前 gRPC request message 绑定到业务 DTO
// - `ShouldBindStream`：把单条流消息绑定到业务 DTO
type GRPCContext interface {
	stdctx.Context
	HandlerContext() *runtime.HandlerContext
	Metadata() Metadata
	FullMethod() FullMethod
	Service() Service
	Method() Method
	Header(key string) string
	ShouldBindReq(obj any) error
	ShouldBindStream(msg any, obj any) error
}

// grpcContext 是 `binding/grpc.GRPCContext` 的默认实现。
// 它是 runtime.HandlerContext 在 gRPC 语义下的一层薄包装。
type grpcContext struct {
	rctx *runtime.HandlerContext
}

var (
	defaultValidatorOnce sync.Once
	defaultValidator     Validator
)

// NewGRPCContext 基于 runtime.HandlerContext 创建一个 gRPC 上下文包装。
func NewGRPCContext(rctx *runtime.HandlerContext) (GRPCContext, error) {
	if rctx == nil {
		return nil, fmt.Errorf("nil runtime context")
	}
	return &grpcContext{rctx: rctx}, nil
}

// HandlerContext 返回底层 runtime.HandlerContext。
func (c *grpcContext) HandlerContext() *runtime.HandlerContext { return c.rctx }

// Deadline 转发到底层 context.Context。
func (c *grpcContext) Deadline() (deadline time.Time, ok bool) {
	return c.baseContext().Deadline()
}

// Done 转发到底层 context.Context。
func (c *grpcContext) Done() <-chan struct{} { return c.baseContext().Done() }

// Err 转发到底层 context.Context。
func (c *grpcContext) Err() error { return c.baseContext().Err() }

// Value 转发到底层 context.Context。
func (c *grpcContext) Value(key any) any { return c.baseContext().Value(key) }

// Metadata 返回 incoming metadata 的快照副本。
func (c *grpcContext) Metadata() Metadata {
	md := map[string][]string(incomingMetadata(c.rctx))
	out := make(map[string][]string, len(md))
	for k, v := range md {
		out[k] = append([]string{}, v...)
	}
	return Metadata(out)
}

// FullMethod 返回当前 RPC 的完整方法名。
func (c *grpcContext) FullMethod() FullMethod {
	value, err := resolveFullMethod(c.rctx, runtime.WrapperDescriptor{})
	if err != nil {
		return FullMethod("")
	}
	if out, ok := value.(FullMethod); ok {
		return out
	}
	return FullMethod("")
}

// Service 返回当前 RPC 的 service 名。
func (c *grpcContext) Service() Service {
	value, err := resolveService(c.rctx, runtime.WrapperDescriptor{})
	if err != nil {
		return Service("")
	}
	if out, ok := value.(Service); ok {
		return out
	}
	return Service("")
}

// Method 返回当前 RPC 的方法名。
func (c *grpcContext) Method() Method {
	value, err := resolveMethod(c.rctx, runtime.WrapperDescriptor{})
	if err != nil {
		return Method("")
	}
	if out, ok := value.(Method); ok {
		return out
	}
	return Method("")
}

// Header 返回指定 metadata key 的首个值。
func (c *grpcContext) Header(key string) string {
	if strings.TrimSpace(key) == "" {
		return ""
	}
	md := incomingMetadata(c.rctx)
	values := md.Get(strings.ToLower(key))
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// ShouldBindReq 把当前 gRPC request message 绑定到 obj，并执行 validate 校验。
func (c *grpcContext) ShouldBindReq(obj any) error {
	return BindReqInto(c.rctx, obj)
}

// ShouldBindStream 把一条流消息绑定到 obj，并执行 validate 校验。
func (c *grpcContext) ShouldBindStream(msg any, obj any) error {
	return BindStreamInto(c.rctx, msg, obj)
}

// BindReqInto 把当前 gRPC request message 绑定到 obj。
//
// 它与 HTTP 的 `ShouldBindJSON` 不同：
// - 不从原始 HTTP body 做内容协商
// - 而是把 transport 已经解码好的 request message 映射到业务 DTO
func BindReqInto(ctx *runtime.HandlerContext, obj any) error {
	if ctx == nil {
		return fmt.Errorf("grpc request binding requires handler context")
	}
	if err := ensureModeAllowed(ctx, "ShouldBindReq", "unary", "server_stream"); err != nil {
		return err
	}
	if ctx.Request == nil || ctx.Request.Body == nil {
		return fmt.Errorf("grpc request message is empty")
	}
	return bindMessageInto(ctx, ctx.Request.Body, obj, "grpc request")
}

// BindStreamInto 把一条流消息绑定到 obj。
//
// 该方法用于 client-stream / bidi-stream 场景：
// - 业务先通过 `Recv()` 读出一条消息
// - 再显式调用 `ShouldBindStream(msg, &dto)` 做 DTO 绑定与校验
func BindStreamInto(ctx *runtime.HandlerContext, msg any, obj any) error {
	if ctx == nil {
		return fmt.Errorf("grpc stream binding requires handler context")
	}
	if err := ensureModeAllowed(ctx, "ShouldBindStream", "client_stream", "bidi_stream"); err != nil {
		return err
	}
	if msg == nil {
		return fmt.Errorf("grpc stream message is empty")
	}
	return bindMessageInto(ctx, msg, obj, "grpc stream message")
}

func matchGRPCContext(t reflect.Type) (runtime.WrapperDescriptor, bool) {
	contextType := reflect.TypeFor[GRPCContext]()
	if t == contextType {
		return runtime.WrapperDescriptor{Kind: "GRPCContext", WrapperType: t, InnerType: t}, true
	}
	return runtime.WrapperDescriptor{}, false
}

func resolveGRPCContext(ctx *runtime.HandlerContext, desc runtime.WrapperDescriptor) (any, error) {
	if existing, ok := ctx.Get(MetadataKeyContext); ok {
		if gc, ok := existing.(GRPCContext); ok && gc != nil {
			return gc, nil
		}
	}
	gc, err := NewGRPCContext(ctx)
	if err != nil {
		return nil, err
	}
	ctx.Set(MetadataKeyContext, gc)
	return gc, nil
}

func marshalRequestMessage(raw any) ([]byte, error) {
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case []byte:
		return v, nil
	case json.RawMessage:
		return []byte(v), nil
	}
	if msg, ok := raw.(proto.Message); ok {
		data, err := protojson.Marshal(msg)
		if err != nil {
			return nil, fmt.Errorf("cannot marshal grpc proto request %T: %w", raw, err)
		}
		return data, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("cannot marshal grpc request %T: %w", raw, err)
	}
	return data, nil
}

func bindMessageInto(ctx *runtime.HandlerContext, raw any, obj any, label string) error {
	if obj == nil {
		return fmt.Errorf("%s binding requires non-nil target", label)
	}
	rv := reflect.ValueOf(obj)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("%s expects non-nil pointer target, got %T", label, obj)
	}
	data, err := marshalRequestMessage(raw)
	if err != nil {
		return err
	}
	if len(data) != 0 {
		if err := json.Unmarshal(data, obj); err != nil {
			return fmt.Errorf("cannot bind %s into %T: %w", label, obj, err)
		}
	}
	v := validatorFromContext(ctx)
	if v == nil {
		v = defaultBindValidator()
	}
	if v == nil {
		return nil
	}
	return v.Validate(obj)
}

func validatorFromContext(ctx *runtime.HandlerContext) Validator {
	if ctx == nil {
		return nil
	}
	if vAny, ok := ctx.Get(MetadataKeyValidator); ok && vAny != nil {
		if v, ok := vAny.(Validator); ok && v != nil {
			return v
		}
	}
	return nil
}

func defaultBindValidator() Validator {
	defaultValidatorOnce.Do(func() {
		defaultValidator = NewValidatorV10()
	})
	return defaultValidator
}

func (c *grpcContext) baseContext() stdctx.Context {
	if c == nil || c.rctx == nil || c.rctx.Context() == nil {
		return stdctx.Background()
	}
	return c.rctx.Context()
}

// V10Validator 将 `github.com/go-playground/validator/v10` 适配为 `grpc.Validator`。
type V10Validator struct {
	v *validator.Validate
}

// NewValidatorV10 创建一个基于 validator/v10 的校验器实现。
func NewValidatorV10() *V10Validator {
	v := validator.New()
	return &V10Validator{v: v}
}

// Validate 对 obj 执行结构体校验，并将校验错误归一化为 KernelError。
func (x *V10Validator) Validate(obj any) error {
	if x == nil || x.v == nil || obj == nil {
		return nil
	}
	if err := x.v.Struct(obj); err != nil {
		return normalizeV10Error(err)
	}
	return nil
}

func normalizeV10Error(err error) error {
	if err == nil {
		return nil
	}
	if inv, ok := err.(*validator.InvalidValidationError); ok {
		return &kerrors.KernelError{
			Kind:    kerrors.KindValidation,
			Message: inv.Error(),
			Cause:   err,
			Meta: map[string]any{
				"source": "validator/v10",
			},
		}
	}
	if ves, ok := err.(validator.ValidationErrors); ok {
		fields := make([]map[string]any, 0, len(ves))
		for _, fe := range ves {
			fields = append(fields, map[string]any{
				"field": fe.Field(),
				"tag":   fe.Tag(),
				"param": fe.Param(),
				"value": fe.Value(),
			})
		}
		msg := "validation failed"
		if len(ves) > 0 {
			first := ves[0]
			msg = fmt.Sprintf("validation failed on %s (%s)", first.Field(), first.Tag())
		}
		return &kerrors.KernelError{
			Kind:    kerrors.KindValidation,
			Message: msg,
			Cause:   err,
			Meta: map[string]any{
				"source": "validator/v10",
				"errors": fields,
			},
		}
	}
	return err
}
