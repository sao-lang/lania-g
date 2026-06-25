package graphql

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// BindArgsInto 将当前 GraphQL 字段的 args 绑定到 obj。
//
// 该入口面向“把字段参数整体映射成一个业务 DTO”的场景。
// 与 `Arg[T]` 的差别在于：这里会把整个 args map 作为一个输入对象来处理。
func BindArgsInto(ctx *runtime.HandlerContext, obj any) error {
	if ctx == nil {
		return fmt.Errorf("graphql args binding requires handler context")
	}
	args, _ := extractArgs(ctx)
	return bindArgsValueInto(args, obj)
}

// ShouldBindArgs 将当前字段 args 绑定到 obj，并在可用时执行 DTO 校验。
func (c *GraphQLContext) ShouldBindArgs(obj any) error {
	if c == nil {
		return fmt.Errorf("graphql context is nil")
	}
	if c.rctx != nil {
		if err := BindArgsInto(c.rctx, obj); err != nil {
			return err
		}
		if v := validatorFromContext(c.rctx); v != nil {
			return v.Validate(obj)
		}
	} else {
		if err := bindArgsValueInto(c.rawArgs, obj); err != nil {
			return err
		}
	}
	if v := defaultBindValidator(); v != nil {
		return v.Validate(obj)
	}
	return nil
}

func bindArgsValueInto(args map[string]any, obj any) error {
	if obj == nil {
		return fmt.Errorf("graphql args binding requires non-nil target")
	}
	rv := reflect.ValueOf(obj)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("graphql args binding expects non-nil pointer target, got %T", obj)
	}
	payload := map[string]any{}
	for k, v := range args {
		payload[k] = v
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("cannot marshal graphql args: %w", err)
	}
	if err := json.Unmarshal(data, obj); err != nil {
		return fmt.Errorf("cannot bind graphql args into %T: %w", obj, err)
	}
	return nil
}
