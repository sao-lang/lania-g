package graphql

import (
	"fmt"
	"testing"

	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	gqlprotocol "github.com/sao-lang/lania-g/protocol/graphql/v3/protocol"
)

type createUserArgsDTO struct {
	Name string `json:"name" validate:"required,min=2"`
	Age  int    `json:"age" validate:"gt=0"`
}

type stubValidator struct {
	err error
}

func (v stubValidator) Validate(obj any) error { return v.err }

func TestGraphQLContext_ShouldBindArgs(t *testing.T) {
	rctx := runtime.NewHandlerContext(gqlprotocol.Protocol)
	rctx.Set(MetadataKeyField, map[string]any{"name": "alice", "age": 18})

	gctx := &GraphQLContext{}
	InitContext(gctx, rctx.Context(), nil, nil, "", "", "Mutation", "createUser", nil, nil, nil, nil, nil, nil, nil, nil, map[string]any{"name": "alice", "age": 18})
	gctx.AttachHandlerContext(rctx)

	var dto createUserArgsDTO
	if err := gctx.ShouldBindArgs(&dto); err != nil {
		t.Fatalf("ShouldBindArgs() error = %v", err)
	}
	if dto.Name != "alice" || dto.Age != 18 {
		t.Fatalf("unexpected dto: %#v", dto)
	}
}

func TestGraphQLContext_ShouldBindArgsValidationError(t *testing.T) {
	rctx := runtime.NewHandlerContext(gqlprotocol.Protocol)
	rctx.Set(MetadataKeyField, map[string]any{"name": "", "age": 0})

	gctx := &GraphQLContext{}
	InitContext(gctx, rctx.Context(), nil, nil, "", "", "Mutation", "createUser", nil, nil, nil, nil, nil, nil, nil, nil, map[string]any{"name": "", "age": 0})
	gctx.AttachHandlerContext(rctx)

	var dto createUserArgsDTO
	err := gctx.ShouldBindArgs(&dto)
	if err == nil {
		t.Fatalf("ShouldBindArgs() expected validation error")
	}
	if ke, ok := err.(*kerrors.KernelError); !ok || ke.Kind != kerrors.KindValidation {
		t.Fatalf("ShouldBindArgs() error kind = %#v, want validation kernel error", err)
	}
}

func TestGraphQLContext_ShouldBindArgsUsesContextValidator(t *testing.T) {
	rctx := runtime.NewHandlerContext(gqlprotocol.Protocol)
	rctx.Set(MetadataKeyField, map[string]any{"name": "alice", "age": 18})
	rctx.Set(MetadataKeyValidator, stubValidator{err: fmt.Errorf("custom validator")})

	gctx := &GraphQLContext{}
	InitContext(gctx, rctx.Context(), nil, nil, "", "", "Mutation", "createUser", nil, nil, nil, nil, nil, nil, nil, nil, map[string]any{"name": "alice", "age": 18})
	gctx.AttachHandlerContext(rctx)

	var dto createUserArgsDTO
	err := gctx.ShouldBindArgs(&dto)
	if err == nil || err.Error() != "custom validator" {
		t.Fatalf("ShouldBindArgs() error = %v, want custom validator", err)
	}
}
