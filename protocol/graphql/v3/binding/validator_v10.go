package graphql

import (
	"fmt"
	"sync"

	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"

	"github.com/go-playground/validator/v10"
)

// Validator 定义 GraphQL 参数 DTO 的校验接口。
type Validator interface {
	Validate(obj any) error
}

// V10Validator 将 `github.com/go-playground/validator/v10` 适配为 `graphql.Validator`。
type V10Validator struct {
	v *validator.Validate
}

var (
	defaultValidatorOnce sync.Once
	defaultValidator     Validator
)

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
