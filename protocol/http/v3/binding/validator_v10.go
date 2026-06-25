// validator_v10.go 对接 go-playground/validator/v10 的 HTTP 参数校验能力。
package http

import (
	"fmt"

	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"

	"github.com/go-playground/validator/v10"
)

// V10Validator 将 `github.com/go-playground/validator/v10` 适配为 `http.Validator`。
//
// 说明：
// - 使用 validator 默认 tag：`validate:"..."`。
// - 本 binding 层也额外支持 gin 风格的 `binding:"required"` 必填标签。
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

// normalizeV10Error 把 validator/v10 的错误收敛成框架统一的 KernelError。
// 这样上层 HTTP adapter 可以继续沿用统一错误出口，不必感知第三方校验器细节。
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
			// message 只取第一条失败项做摘要，完整细节仍放进 Meta.errors。
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

	// Unknown error: keep as-is.
	return err
}
