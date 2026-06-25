package aop

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	en_translations "github.com/go-playground/validator/v10/translations/en"
)

// PipeTransform 定义 Pipe 的核心行为：把输入值转换为目标值，或在失败时返回错误。
type PipeTransform interface {
	Transform(value interface{}, metadata interface{}) (interface{}, error)
}

// Pipe 是框架对“数据转换/校验器”对象的统一抽象。
type Pipe interface {
	PipeTransform
}

// PipeConstructor 用于延迟创建 Pipe 实例。
type PipeConstructor func() Pipe

// PipeFunc 是 Pipe 的函数式写法。
type PipeFunc func(value interface{}, metadata interface{}) (interface{}, error)

// Transform 让 PipeFunc 适配 Pipe 接口。
//
// value 是待处理的值（入参或返回值），metadata 是上层传入的元信息（通常是 *ArgumentMetadata）。
func (f PipeFunc) Transform(value interface{}, metadata interface{}) (interface{}, error) {
	return f(value, metadata)
}

// WrapPipe 将 Pipe（对象形式）包装为 PipeFunc（函数形式）。
//
// 这样上层 pipeline 可以统一用函数切片执行 pipes，减少接口调用与分配。
func WrapPipe(pipe Pipe) PipeFunc {
	return func(value interface{}, metadata interface{}) (interface{}, error) {
		return pipe.Transform(value, metadata)
	}
}

// ArgumentMetadata 描述当前正在处理的参数元信息。
type ArgumentMetadata struct {
	Type     reflect.Type
	Data     string
	Metatype reflect.Type
}

// ValidationError 表示单个字段的校验错误。
type ValidationError struct {
	Field   string
	Message string
}

// Error 实现 error 接口，输出 `field: message`。
func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationException 表示一组校验错误的聚合结果。
type ValidationException struct {
	Message string
	Errors  []ValidationError
}

// Error 实现 error 接口；当包含多条字段错误时，会拼接为多行文本，便于日志/响应输出。
func (e *ValidationException) Error() string {
	if len(e.Errors) == 0 {
		return e.Message
	}
	var sb strings.Builder
	sb.WriteString(e.Message)
	for _, err := range e.Errors {
		sb.WriteString(fmt.Sprintf("\n  - %s", err.Error()))
	}
	return sb.String()
}

// NewValidationException 创建一个 ValidationException。
//
// 通常由 ValidationPipe 或业务自定义 pipe 用于汇总多字段校验错误。
func NewValidationException(message string, errors ...ValidationError) *ValidationException {
	return &ValidationException{
		Message: message,
		Errors:  errors,
	}
}

// ParseIntPipe 是一个把输入转换为 int 的内置 Pipe。
type ParseIntPipe struct {
	ErrorHttpStatus  int
	ExceptionFactory func(error) error
}

// NewParseIntPipe 创建一个用于“将输入解析为 int”的 Pipe。
//
// 典型用法：把 query/path param 的 string 转为 int。
func NewParseIntPipe() *ParseIntPipe {
	return &ParseIntPipe{
		ErrorHttpStatus: 400,
	}
}

// Transform 将 value 解析为 int。
//
// 支持的输入：
// - string：按十进制解析
// - int/int64/float64：做必要的转换
// - 其他类型：fmt.Sprintf("%v") 转为字符串后再解析
//
// 失败时：
// - 若配置了 ExceptionFactory，使用它生成错误
// - 否则返回通用的 validation failed 错误
func (p *ParseIntPipe) Transform(value interface{}, metadata interface{}) (interface{}, error) {
	if value == nil {
		return nil, errors.New("value is required")
	}

	var str string
	switch v := value.(type) {
	case string:
		str = v
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	default:
		str = fmt.Sprintf("%v", v)
	}

	result, err := strconv.Atoi(str)
	if err != nil {
		if p.ExceptionFactory != nil {
			return nil, p.ExceptionFactory(err)
		}
		return nil, fmt.Errorf("validation failed (integer string is not an integer)")
	}

	return result, nil
}

// ParseBoolPipe 是一个把输入转换为 bool 的内置 Pipe。
type ParseBoolPipe struct {
	ErrorHttpStatus  int
	ExceptionFactory func(error) error
}

// NewParseBoolPipe 创建一个用于“将输入解析为 bool”的 Pipe。
func NewParseBoolPipe() *ParseBoolPipe {
	return &ParseBoolPipe{
		ErrorHttpStatus: 400,
	}
}

// Transform 将 value 解析为 bool。
//
// 支持的输入：
// - bool：原样返回
// - string：大小写不敏感，支持 true/false 及 1/0、yes/no、on/off
func (p *ParseBoolPipe) Transform(value interface{}, metadata interface{}) (interface{}, error) {
	if value == nil {
		return nil, errors.New("value is required")
	}

	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		lower := strings.ToLower(v)
		if lower == "true" || lower == "1" || lower == "yes" || lower == "on" {
			return true, nil
		}
		if lower == "false" || lower == "0" || lower == "no" || lower == "off" {
			return false, nil
		}
	}

	if p.ExceptionFactory != nil {
		return nil, p.ExceptionFactory(fmt.Errorf("validation failed (boolean string is not a boolean)"))
	}
	return nil, fmt.Errorf("validation failed (boolean string is not a boolean)")
}

// ParseFloatPipe 是一个把输入转换为 float64 的内置 Pipe。
type ParseFloatPipe struct {
	ErrorHttpStatus  int
	ExceptionFactory func(error) error
}

// NewParseFloatPipe 创建一个用于“将输入解析为 float64”的 Pipe。
func NewParseFloatPipe() *ParseFloatPipe {
	return &ParseFloatPipe{
		ErrorHttpStatus: 400,
	}
}

// Transform 将 value 解析为 float64。
//
// 支持的输入：
// - string：按 strconv.ParseFloat(str, 64) 解析
// - float64/int/int64：做必要转换
// - 其他类型：fmt.Sprintf("%v") 后解析
func (p *ParseFloatPipe) Transform(value interface{}, metadata interface{}) (interface{}, error) {
	if value == nil {
		return nil, errors.New("value is required")
	}

	var str string
	switch v := value.(type) {
	case string:
		str = v
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	default:
		str = fmt.Sprintf("%v", v)
	}

	result, err := strconv.ParseFloat(str, 64)
	if err != nil {
		if p.ExceptionFactory != nil {
			return nil, p.ExceptionFactory(err)
		}
		return nil, fmt.Errorf("validation failed (float string is not a float)")
	}

	return result, nil
}

// DefaultValuePipe 会在输入为空时回退到默认值。
type DefaultValuePipe struct {
	defaultValue interface{}
}

// NewDefaultValuePipe 创建一个默认值 Pipe：
// - 当输入 value 为 nil（或某些“空值”）时，返回 defaultValue
// - 否则原样返回 value
func NewDefaultValuePipe(defaultValue interface{}) *DefaultValuePipe {
	return &DefaultValuePipe{
		defaultValue: defaultValue,
	}
}

// Transform 应用默认值逻辑：
// - value == nil：返回默认值
// - value 是 string 且为空串：返回默认值
func (p *DefaultValuePipe) Transform(value interface{}, metadata interface{}) (interface{}, error) {
	if value == nil {
		return p.defaultValue, nil
	}

	switch v := value.(type) {
	case string:
		if v == "" {
			return p.defaultValue, nil
		}
	}

	return value, nil
}

// ParseArrayPipe 是一个把输入转换为数组的内置 Pipe。
type ParseArrayPipe struct {
	Separator string
	Optional  bool
}

// NewParseArrayPipe 创建一个用于“将输入解析为数组”的 Pipe。
//
// 默认使用 `,` 分隔；Optional=false 表示 value=nil 时返回错误。
func NewParseArrayPipe() *ParseArrayPipe {
	return &ParseArrayPipe{
		Separator: ",",
		Optional:  false,
	}
}

// Transform 将输入解析为 []interface{}。
//
// 支持的输入：
// - []interface{}：原样返回
// - []string：逐个转为 interface{}
// - string：按 Separator split 并 trim 空白
// - 其他：包装成单元素数组
func (p *ParseArrayPipe) Transform(value interface{}, metadata interface{}) (interface{}, error) {
	if value == nil {
		if p.Optional {
			return []interface{}{}, nil
		}
		return nil, errors.New("value is required")
	}

	if arr, ok := value.([]interface{}); ok {
		return arr, nil
	}

	if arr, ok := value.([]string); ok {
		result := make([]interface{}, len(arr))
		for i, v := range arr {
			result[i] = v
		}
		return result, nil
	}

	if str, ok := value.(string); ok {
		parts := strings.Split(str, p.Separator)
		result := make([]interface{}, len(parts))
		for i, part := range parts {
			result[i] = strings.TrimSpace(part)
		}
		return result, nil
	}

	return []interface{}{value}, nil
}

// ValidationPipe 是一个基于 validator 的结构体验证 Pipe。
type ValidationPipe struct {
	Whitelist             bool
	ForbidNonWhitelisted  bool
	ForbidUnknownValues   bool
	SkipMissingProperties bool
	ExceptionFactory      func([]ValidationError) error
	validate              *validator.Validate
	trans                 ut.Translator
}

// NewValidationPipe 创建一个基于 go-playground/validator 的结构体验证 Pipe。
//
// 行为概览：
// - 仅当 value 是 struct 或 *struct 时才验证；其他类型原样返回
// - 使用 json tag 作为字段名（更贴近对外 API）
// - 默认使用英文翻译（en_translations）
func NewValidationPipe() *ValidationPipe {
	validate := validator.New()

	en := en.New()
	uni := ut.New(en, en)
	trans, _ := uni.GetTranslator("en")

	en_translations.RegisterDefaultTranslations(validate, trans)

	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	return &ValidationPipe{
		Whitelist:             true,
		ForbidNonWhitelisted:  false,
		ForbidUnknownValues:   false,
		SkipMissingProperties: false,
		validate:              validate,
		trans:                 trans,
	}
}

// Transform 对结构体执行校验，并在失败时返回错误。
//
// 错误格式：
// - 若 err 是 validator.ValidationErrors，会转换为 []ValidationError
// - 若配置了 ExceptionFactory，则用它生成错误（便于自定义错误模型）
func (p *ValidationPipe) Transform(value interface{}, metadata interface{}) (interface{}, error) {
	if value == nil {
		return value, nil
	}

	val := reflect.ValueOf(value)
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return value, nil
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return value, nil
	}

	if err := p.validate.Struct(value); err != nil {
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			var customErrors []ValidationError
			for _, e := range validationErrors {
				customErrors = append(customErrors, ValidationError{
					Field:   e.Field(),
					Message: e.Translate(p.trans),
				})
			}
			if p.ExceptionFactory != nil {
				return nil, p.ExceptionFactory(customErrors)
			}
			var errMsgs []string
			for _, e := range customErrors {
				errMsgs = append(errMsgs, e.Error())
			}
			return nil, fmt.Errorf("validation failed: %s", strings.Join(errMsgs, "; "))
		}
		return nil, err
	}

	return value, nil
}

// ParseUUIDPipe 是一个用于校验 UUID 字符串格式的内置 Pipe。
type ParseUUIDPipe struct {
	ErrorHttpStatus  int
	ExceptionFactory func(error) error
	Version          string
}

// NewParseUUIDPipe 创建一个用于“校验 UUID 字符串格式”的 Pipe。
//
// 注意：当前实现只做固定长度与 `-` 位置检查，并不校验版本号与十六进制字符集合。
func NewParseUUIDPipe() *ParseUUIDPipe {
	return &ParseUUIDPipe{
		ErrorHttpStatus: 400,
	}
}

// Transform 校验输入是否为 UUID 格式字符串，失败返回 validation failed 错误（或 ExceptionFactory 生成的错误）。
func (p *ParseUUIDPipe) Transform(value interface{}, metadata interface{}) (interface{}, error) {
	if value == nil {
		return nil, errors.New("value is required")
	}

	var str string
	switch v := value.(type) {
	case string:
		str = v
	default:
		str = fmt.Sprintf("%v", v)
	}

	if !isValidUUID(str) {
		if p.ExceptionFactory != nil {
			return nil, p.ExceptionFactory(fmt.Errorf("validation failed (UUID is not a UUID)"))
		}
		return nil, fmt.Errorf("validation failed (UUID is not a UUID)")
	}

	return str, nil
}

// isValidUUID 做一个轻量级 UUID 形态校验：
// - 长度必须为 36
// - 分隔符 `-` 必须出现在固定位置
func isValidUUID(uuid string) bool {
	if len(uuid) != 36 {
		return false
	}
	if uuid[8] != '-' || uuid[13] != '-' || uuid[18] != '-' || uuid[23] != '-' {
		return false
	}
	return true
}
