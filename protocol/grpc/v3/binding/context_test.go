package grpc

import (
	"reflect"
	"testing"

	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	grpcprotocol "github.com/sao-lang/lania-g/protocol/grpc/v3/protocol"
	"google.golang.org/protobuf/types/known/structpb"
)

type createUserDTO struct {
	Name  string `json:"name" validate:"required,min=2"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gt=0"`
}

func TestGRPCContextShouldBindReq(t *testing.T) {
	req := mustStructPB(t, map[string]any{
		"name":  "alice",
		"email": "alice@example.com",
		"age":   18,
	})
	rctx := runtime.NewHandlerContext(grpcprotocol.Protocol)
	rctx.Request.Body = req
	rctx.Set(MetadataKeyMode, "unary")

	gctx, err := NewGRPCContext(rctx)
	if err != nil {
		t.Fatalf("NewGRPCContext() error = %v", err)
	}

	var dto createUserDTO
	if err := gctx.ShouldBindReq(&dto); err != nil {
		t.Fatalf("ShouldBindReq() error = %v", err)
	}
	if dto.Name != "alice" || dto.Email != "alice@example.com" || dto.Age != 18 {
		t.Fatalf("unexpected dto: %#v", dto)
	}
}

func TestGRPCContextShouldBindReqValidationError(t *testing.T) {
	req := mustStructPB(t, map[string]any{
		"name":  "",
		"email": "invalid-email",
		"age":   0,
	})
	rctx := runtime.NewHandlerContext(grpcprotocol.Protocol)
	rctx.Request.Body = req
	rctx.Set(MetadataKeyMode, "unary")

	gctx, err := NewGRPCContext(rctx)
	if err != nil {
		t.Fatalf("NewGRPCContext() error = %v", err)
	}

	var dto createUserDTO
	err = gctx.ShouldBindReq(&dto)
	if err == nil {
		t.Fatalf("ShouldBindReq() expected validation error")
	}
	if ke, ok := err.(*kerrors.KernelError); !ok || ke.Kind != kerrors.KindValidation {
		t.Fatalf("ShouldBindReq() error kind = %#v, want validation kernel error", err)
	}
}

func TestGRPCContextShouldBindStream(t *testing.T) {
	msg := mustStructPB(t, map[string]any{
		"name":  "bob",
		"email": "bob@example.com",
		"age":   24,
	})
	rctx := runtime.NewHandlerContext(grpcprotocol.Protocol)
	rctx.Set(MetadataKeyMode, "client_stream")

	gctx, err := NewGRPCContext(rctx)
	if err != nil {
		t.Fatalf("NewGRPCContext() error = %v", err)
	}

	var dto createUserDTO
	if err := gctx.ShouldBindStream(msg, &dto); err != nil {
		t.Fatalf("ShouldBindStream() error = %v", err)
	}
	if dto.Name != "bob" || dto.Email != "bob@example.com" || dto.Age != 24 {
		t.Fatalf("unexpected dto: %#v", dto)
	}
}

func TestGRPCContextShouldBindStreamValidationError(t *testing.T) {
	msg := mustStructPB(t, map[string]any{
		"name":  "x",
		"email": "invalid-email",
		"age":   0,
	})
	rctx := runtime.NewHandlerContext(grpcprotocol.Protocol)
	rctx.Set(MetadataKeyMode, "bidi_stream")

	gctx, err := NewGRPCContext(rctx)
	if err != nil {
		t.Fatalf("NewGRPCContext() error = %v", err)
	}

	var dto createUserDTO
	err = gctx.ShouldBindStream(msg, &dto)
	if err == nil {
		t.Fatalf("ShouldBindStream() expected validation error")
	}
	if ke, ok := err.(*kerrors.KernelError); !ok || ke.Kind != kerrors.KindValidation {
		t.Fatalf("ShouldBindStream() error kind = %#v, want validation kernel error", err)
	}
}

func TestResolveGRPCContext(t *testing.T) {
	rctx := runtime.NewHandlerContext(grpcprotocol.Protocol)
	rctx.Set(MetadataKeyMode, "unary")
	desc, ok := matchGRPCContext(reflect.TypeFor[GRPCContext]())
	if !ok {
		t.Fatalf("matchGRPCContext() = false")
	}
	value, err := resolveGRPCContext(rctx, desc)
	if err != nil {
		t.Fatalf("resolveGRPCContext() error = %v", err)
	}
	if _, ok := value.(GRPCContext); !ok {
		t.Fatalf("resolveGRPCContext() returned %T, want GRPCContext", value)
	}
}

func mustStructPB(t *testing.T, data map[string]any) *structpb.Struct {
	t.Helper()
	out, err := structpb.NewStruct(data)
	if err != nil {
		t.Fatalf("structpb.NewStruct() error = %v", err)
	}
	return out
}
