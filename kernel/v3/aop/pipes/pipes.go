// Package pipes 提供一组常用的内置 Pipe。
//
// 它们与 kernel/v3/aop 内置的 Parse* / ValidationPipe 互补，覆盖最常见的
// "入参清洗 + 格式校验" 场景。所有 Pipe 都同时支持对象与函数两种用法：
//
//	pipes.NewTrimSpacePipe()           // 对象
//	func(v, _ interface{}) (interface{}, error) { ... }  // 函数
package pipes

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

// TrimSpacePipe 去掉字符串两端的空白。
//
// 支持 string、[]string 与结构体中的导出字符串字段（非指针字段原地修改副本）。
type TrimSpacePipe struct{}

// NewTrimSpacePipe 创建一个去空白 Pipe。
func NewTrimSpacePipe() *TrimSpacePipe { return &TrimSpacePipe{} }

// Transform 实现 Pipe 接口。
func (p *TrimSpacePipe) Transform(value interface{}, _ interface{}) (interface{}, error) {
	return trimValue(value), nil
}

func trimValue(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []string:
		out := make([]string, len(v))
		for i, item := range v {
			out[i] = strings.TrimSpace(item)
		}
		return out
	}

	rv := reflect.ValueOf(value)
	if !rv.IsValid() || rv.Kind() != reflect.Struct {
		return value
	}
	out := reflect.New(rv.Type()).Elem()
	out.Set(rv)
	for i := 0; i < out.NumField(); i++ {
		field := out.Field(i)
		if !field.CanSet() || field.Kind() != reflect.String {
			continue
		}
		field.SetString(strings.TrimSpace(field.String()))
	}
	return out.Interface()
}

// ParseTimePipe 把字符串解析为 time.Time。
//
// Layout 为空时依次尝试 RFC3339、`2006-01-02 15:04:05` 与 `2006-01-02`。
type ParseTimePipe struct {
	Layout   string
	Location *time.Location
}

// NewParseTimePipe 创建一个时间解析 Pipe；layout 为空时使用内置的常见格式。
func NewParseTimePipe(layout string) *ParseTimePipe {
	return &ParseTimePipe{Layout: layout}
}

var defaultTimeLayouts = []string{
	time.RFC3339,
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// Transform 实现 Pipe 接口。
func (p *ParseTimePipe) Transform(value interface{}, _ interface{}) (interface{}, error) {
	if _, ok := value.(time.Time); ok {
		return value, nil
	}
	text, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("ParseTimePipe expects string, got %T", value)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("ParseTimePipe requires a non-empty value")
	}

	layouts := defaultTimeLayouts
	if p != nil && p.Layout != "" {
		layouts = []string{p.Layout}
	}
	location := time.UTC
	if p != nil && p.Location != nil {
		location = p.Location
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, text, location); err == nil {
			return parsed, nil
		}
	}
	return nil, fmt.Errorf("ParseTimePipe cannot parse %q", text)
}

// ParseEnumPipe 校验值是否在允许集合内。
//
// 返回的是原值（不做类型转换），适合放在业务 handler 之前做白名单校验。
type ParseEnumPipe struct {
	Allowed    []string
	IgnoreCase bool
}

// NewParseEnumPipe 创建一个枚举校验 Pipe。
func NewParseEnumPipe(allowed ...string) *ParseEnumPipe {
	return &ParseEnumPipe{Allowed: allowed, IgnoreCase: true}
}

// Transform 实现 Pipe 接口。
func (p *ParseEnumPipe) Transform(value interface{}, _ interface{}) (interface{}, error) {
	if p == nil || len(p.Allowed) == 0 {
		return value, nil
	}
	text := fmt.Sprint(value)
	for _, allowed := range p.Allowed {
		if p.IgnoreCase {
			if strings.EqualFold(text, allowed) {
				return value, nil
			}
			continue
		}
		if text == allowed {
			return value, nil
		}
	}
	return nil, fmt.Errorf("value %q is not one of [%s]", text, strings.Join(p.Allowed, ", "))
}

// RequiredPipe 拒绝空值。
//
// 判空规则：nil、空字符串、长度 0 的 slice/map、全零结构体。
type RequiredPipe struct {
	Message string
}

// NewRequiredPipe 创建一个必填校验 Pipe。
func NewRequiredPipe() *RequiredPipe { return &RequiredPipe{} }

// Transform 实现 Pipe 接口。
func (p *RequiredPipe) Transform(value interface{}, _ interface{}) (interface{}, error) {
	if !isEmptyValue(value) {
		return value, nil
	}
	message := "value is required"
	if p != nil && p.Message != "" {
		message = p.Message
	}
	return nil, fmt.Errorf("%s", message)
}

func isEmptyValue(value interface{}) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return rv.IsNil()
	default:
		return rv.IsZero()
	}
}
