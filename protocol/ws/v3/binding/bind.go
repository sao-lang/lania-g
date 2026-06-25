// bind.go 提供 WS 上下文到结构体的绑定辅助。
package ws

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// WS 绑定只支持少量 tag：
// - `header` 从握手头/消息上下文头里取值
// - `body` 从消息体对象中按字段名取值
// - `required` 约束字段必须存在
const (
	TagHeader   = "header"
	TagBody     = "body"
	TagRequired = "required"
	TagJSON     = "json"
)

// BindInto 按 WS 请求头与消息体内容把数据绑定到 obj。
//
// 流程是：
// - 若消息体存在，先尝试把整个 payload 当 JSON 解到 obj
// - 再按字段 tag 覆盖 header/body 指定的局部字段
//
// 这样既兼容“整包 DTO 解码”，也兼容少量字段从 header 单独补值。
func BindInto(ctx *runtime.HandlerContext, obj any) error {
	if ctx == nil || obj == nil {
		return nil
	}
	val := reflect.ValueOf(obj)
	if val.Kind() != reflect.Ptr || val.IsNil() {
		return nil
	}
	elem := val.Elem()
	if elem.Kind() != reflect.Struct {
		return nil
	}

	payload := messageBytes(ctx)
	if len(payload) > 0 {
		// 先做整包 JSON 反序列化，让 DTO 在没有细粒度 tag 时也能直接工作。
		if err := json.Unmarshal(payload, obj); err != nil {
			return fmt.Errorf("invalid json message: %w", err)
		}
	}

	headers := headersFromContext(ctx)

	typ := elem.Type()
	for i := 0; i < elem.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := elem.Field(i)
		if !fieldVal.CanSet() {
			continue
		}
		tag := field.Tag
		required := fieldRequired(tag)

		if key := tag.Get(TagHeader); key != "" {
			raw := headers.Get(key)
			if required && raw == "" {
				return fmt.Errorf("missing required header %q for field %s", key, field.Name)
			}
			if err := setFromRaw(fieldVal, raw); err != nil {
				return fmt.Errorf("invalid header %q for field %s: %w", key, field.Name, err)
			}
			continue
		}

		if key := tag.Get(TagBody); key != "" {
			// `body:"x"` 走消息对象字段读取，适合消息体较大但只想取部分字段的场景。
			raw, ok := messageField(ctx, key)
			if required && (!ok || isZeroRaw(raw)) {
				return fmt.Errorf("missing required body field %q for field %s", key, field.Name)
			}
			if ok {
				if err := setFromRaw(fieldVal, raw); err != nil {
					return fmt.Errorf("invalid body field %q for field %s: %w", key, field.Name, err)
				}
			}
			continue
		}
	}

	return nil
}

// fieldRequired 保持很宽松的兼容语义：只要写了非空的 required 标记，就视为 required。
func fieldRequired(tag reflect.StructTag) bool {
	raw := strings.ToLower(strings.TrimSpace(tag.Get(TagRequired)))
	switch raw {
	case "", "0", "false", "no":
		return false
	default:
		return true
	}
}

// headersFromContext 优先读取 adapter 已经放进 metadata 的 header 快照，
// 没有时再从 runtime.Request.HeadersMulti 构造一个视图。
func headersFromContext(ctx *runtime.HandlerContext) http.Header {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Get(MetadataKeyHeaders); ok {
		if h, ok := v.(http.Header); ok {
			return h
		}
	}
	out := http.Header{}
	for k, v := range ctx.Request.HeadersMulti {
		out[k] = append([]string{}, v...)
	}
	return out
}

// messageBytes 统一把 WS 消息体投影成 `[]byte`。
// 这样 BindInto / messageField 都可以复用同一份原始输入。
func messageBytes(ctx *runtime.HandlerContext) []byte {
	if ctx == nil {
		return nil
	}
	if len(ctx.Request.BodyBytes) > 0 {
		return ctx.Request.BodyBytes
	}
	if b, ok := ctx.Request.Body.([]byte); ok {
		return b
	}
	if s, ok := ctx.Request.Body.(string); ok {
		return []byte(s)
	}
	if ctx.Request.Body == nil {
		return nil
	}
	// 对 map/struct 等消息对象做一次 JSON 编码，给后续按 JSON 处理的路径复用。
	b, err := json.Marshal(ctx.Request.Body)
	if err != nil {
		return nil
	}
	return b
}

// messageField 从消息体里读取单个字段。
// 它优先利用已经是 map 的 body；否则再把消息字节解成 map 做一次惰性字段读取。
func messageField(ctx *runtime.HandlerContext, key string) (any, bool) {
	if ctx == nil || key == "" {
		return nil, false
	}

	if m, ok := ctx.Request.Body.(map[string]any); ok {
		v, ok := m[key]
		return v, ok
	}
	if raw := messageBytes(ctx); len(raw) > 0 {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err == nil {
			v, ok := m[key]
			return v, ok
		}
	}
	return nil, false
}

func isZeroRaw(v any) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	return ok && s == ""
}

// setFromRaw 复用 binding/ws 的统一 decodeTo，把原始值写入字段。
func setFromRaw(fieldVal reflect.Value, raw any) error {
	if !fieldVal.CanSet() {
		return nil
	}
	value, err := decodeTo(fieldVal.Type(), raw)
	if err != nil {
		return err
	}
	if !value.IsValid() {
		return nil
	}
	if value.Type().AssignableTo(fieldVal.Type()) {
		fieldVal.Set(value)
		return nil
	}
	if value.Type().ConvertibleTo(fieldVal.Type()) {
		fieldVal.Set(value.Convert(fieldVal.Type()))
		return nil
	}
	return fmt.Errorf("cannot assign %s to %s", value.Type().String(), fieldVal.Type().String())
}
