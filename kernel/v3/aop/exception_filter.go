package aop

import "net/http"

// ExceptionFilter 定义异常过滤器的核心行为：接收错误并决定是否消费它。
type ExceptionFilter interface {
	Catch(exception interface{}, ctx *ExecutionContext) error
}

// ExceptionFilterConstructor 用于延迟创建 ExceptionFilter 实例。
type ExceptionFilterConstructor func() ExceptionFilter

// ExceptionFilterFunc 是 ExceptionFilter 的函数式写法。
type ExceptionFilterFunc func(exception interface{}, ctx *ExecutionContext) error

// Catch 让 ExceptionFilterFunc 适配 ExceptionFilter 接口。
//
// 语义约定：
// - exception 通常是 error（也可能是 panic 的 recover 值），由上层 pipeline 传入
// - 返回 nil 表示该异常已被处理/消费；返回非 nil 表示将该错误继续向上抛出或被后续 filter 处理
func (f ExceptionFilterFunc) Catch(exception interface{}, ctx *ExecutionContext) error {
	return f(exception, ctx)
}

// HttpException 表示带 HTTP 状态码语义的异常。
// 它通常由 HTTP adapter 读取并映射到最终响应。
type HttpException struct {
	Status  int
	Message string
	Cause   error
	// Headers 会写入最终响应的响应头。
	// 例如 405 需要带 `Allow`，429 通常需要带 `Retry-After`。
	Headers map[string]string
	// Code 是业务错误码；为空时由 adapter 生成 `HTTP-<status>`。
	Code string
	// Details 是结构化细节，例如字段级校验错误列表。
	Details any
}

// Error 实现 error 接口，返回对外可见的错误消息。
func (e *HttpException) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return httpStatusText(e.Status)
}

// Unwrap 暴露底层原因，使 errors.Is / errors.As 可以继续向下匹配。
func (e *HttpException) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// WithHeaders 追加响应头并返回自身，便于链式构造。
func (e *HttpException) WithHeaders(headers map[string]string) *HttpException {
	if e != nil && len(headers) > 0 {
		if e.Headers == nil {
			e.Headers = make(map[string]string, len(headers))
		}
		for key, value := range headers {
			e.Headers[key] = value
		}
	}
	return e
}

// WithCode 设置业务错误码并返回自身。
func (e *HttpException) WithCode(code string) *HttpException {
	if e != nil {
		e.Code = code
	}
	return e
}

// WithDetails 设置结构化细节并返回自身。
func (e *HttpException) WithDetails(details any) *HttpException {
	if e != nil {
		e.Details = details
	}
	return e
}

// WithCause 设置底层原因并返回自身。
func (e *HttpException) WithCause(cause error) *HttpException {
	if e != nil {
		e.Cause = cause
	}
	return e
}

// NewHttpException 创建一个带 HTTP 状态码的异常对象。
//
// 参数顺序与 Go 生态惯例一致（status 在前）：`NewHttpException(404, "not found")`。
// message 为空时使用该状态码的标准文案。
func NewHttpException(status int, message string) *HttpException {
	if message == "" {
		message = httpStatusText(status)
	}
	return &HttpException{Status: status, Message: message}
}

// 以下构造器覆盖最常用的 4xx / 5xx；更完整的列表见 http_error.go。

// BadRequestException 创建 400 异常。
func BadRequestException(message string) *HttpException {
	return NewHttpException(http.StatusBadRequest, message)
}

// UnauthorizedException 创建 401 异常。
func UnauthorizedException(message string) *HttpException {
	return NewHttpException(http.StatusUnauthorized, message)
}

// ForbiddenException 创建 403 异常。
func ForbiddenException(message string) *HttpException {
	return NewHttpException(http.StatusForbidden, message)
}

// NotFoundException 创建 404 异常。
func NotFoundException(message string) *HttpException {
	return NewHttpException(http.StatusNotFound, message)
}

// NotAcceptableException 创建 406 异常。
func NotAcceptableException(message string) *HttpException {
	return NewHttpException(http.StatusNotAcceptable, message)
}

// RequestTimeoutException 创建 408 异常。
func RequestTimeoutException(message string) *HttpException {
	return NewHttpException(http.StatusRequestTimeout, message)
}

// ConflictException 创建 409 异常。
func ConflictException(message string) *HttpException {
	return NewHttpException(http.StatusConflict, message)
}

// GoneException 创建 410 异常。
func GoneException(message string) *HttpException {
	return NewHttpException(http.StatusGone, message)
}

// PayloadTooLargeException 创建 413 异常。
func PayloadTooLargeException(message string) *HttpException {
	return NewHttpException(http.StatusRequestEntityTooLarge, message)
}

// UnsupportedMediaTypeException 创建 415 异常。
func UnsupportedMediaTypeException(message string) *HttpException {
	return NewHttpException(http.StatusUnsupportedMediaType, message)
}

// UnprocessableEntityException 创建 422 异常。
func UnprocessableEntityException(message string) *HttpException {
	return NewHttpException(http.StatusUnprocessableEntity, message)
}

// InternalServerErrorException 创建 500 异常。
func InternalServerErrorException(message string) *HttpException {
	return NewHttpException(http.StatusInternalServerError, message)
}

// NotImplementedException 创建 501 异常。
func NotImplementedException(message string) *HttpException {
	return NewHttpException(http.StatusNotImplemented, message)
}

// BadGatewayException 创建 502 异常。
func BadGatewayException(message string) *HttpException {
	return NewHttpException(http.StatusBadGateway, message)
}

// ServiceUnavailableException 创建 503 异常。
func ServiceUnavailableException(message string) *HttpException {
	return NewHttpException(http.StatusServiceUnavailable, message)
}

// GatewayTimeoutException 创建 504 异常。
func GatewayTimeoutException(message string) *HttpException {
	return NewHttpException(http.StatusGatewayTimeout, message)
}