package aop

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

// NewHttpException 创建一个带 HTTP status 的异常对象。
//
// 该类型主要用于 HTTP 适配器：adapter 可以把 Status 映射到响应码，并把 Message 写入响应体。
func NewHttpException(message string, status int) *HttpException {
	return &HttpException{
		Status:  status,
		Message: message,
	}
}

// BadRequestException 创建 400 Bad Request 异常；message 为空时使用默认文案。
func BadRequestException(message string) *HttpException {
	if message == "" {
		message = "Bad Request"
	}
	return NewHttpException(message, 400)
}

// UnauthorizedException 创建 401 Unauthorized 异常；message 为空时使用默认文案。
func UnauthorizedException(message string) *HttpException {
	if message == "" {
		message = "Unauthorized"
	}
	return NewHttpException(message, 401)
}

// NotFoundException 创建 404 Not Found 异常；message 为空时使用默认文案。
func NotFoundException(message string) *HttpException {
	if message == "" {
		message = "Not Found"
	}
	return NewHttpException(message, 404)
}

// ForbiddenException 创建 403 Forbidden 异常；message 为空时使用默认文案。
func ForbiddenException(message string) *HttpException {
	if message == "" {
		message = "Forbidden"
	}
	return NewHttpException(message, 403)
}

// NotAcceptableException 创建 406 Not Acceptable 异常；message 为空时使用默认文案。
func NotAcceptableException(message string) *HttpException {
	if message == "" {
		message = "Not Acceptable"
	}
	return NewHttpException(message, 406)
}

// RequestTimeoutException 创建 408 Request Timeout 异常；message 为空时使用默认文案。
func RequestTimeoutException(message string) *HttpException {
	if message == "" {
		message = "Request Timeout"
	}
	return NewHttpException(message, 408)
}

// ConflictException 创建 409 Conflict 异常；message 为空时使用默认文案。
func ConflictException(message string) *HttpException {
	if message == "" {
		message = "Conflict"
	}
	return NewHttpException(message, 409)
}

// GoneException 创建 410 Gone 异常；message 为空时使用默认文案。
func GoneException(message string) *HttpException {
	if message == "" {
		message = "Gone"
	}
	return NewHttpException(message, 410)
}

// PayloadTooLargeException 创建 413 Payload Too Large 异常；message 为空时使用默认文案。
func PayloadTooLargeException(message string) *HttpException {
	if message == "" {
		message = "Payload Too Large"
	}
	return NewHttpException(message, 413)
}

// UnsupportedMediaTypeException 创建 415 Unsupported Media Type 异常；message 为空时使用默认文案。
func UnsupportedMediaTypeException(message string) *HttpException {
	if message == "" {
		message = "Unsupported Media Type"
	}
	return NewHttpException(message, 415)
}

// UnprocessableEntityException 创建 422 Unprocessable Entity 异常；message 为空时使用默认文案。
func UnprocessableEntityException(message string) *HttpException {
	if message == "" {
		message = "Unprocessable Entity"
	}
	return NewHttpException(message, 422)
}

// InternalServerErrorException 创建 500 Internal Server Error 异常；message 为空时使用默认文案。
func InternalServerErrorException(message string) *HttpException {
	if message == "" {
		message = "Internal Server Error"
	}
	return NewHttpException(message, 500)
}

// NotImplementedException 创建 501 Not Implemented 异常；message 为空时使用默认文案。
func NotImplementedException(message string) *HttpException {
	if message == "" {
		message = "Not Implemented"
	}
	return NewHttpException(message, 501)
}

// BadGatewayException 创建 502 Bad Gateway 异常；message 为空时使用默认文案。
func BadGatewayException(message string) *HttpException {
	if message == "" {
		message = "Bad Gateway"
	}
	return NewHttpException(message, 502)
}

// ServiceUnavailableException 创建 503 Service Unavailable 异常；message 为空时使用默认文案。
func ServiceUnavailableException(message string) *HttpException {
	if message == "" {
		message = "Service Unavailable"
	}
	return NewHttpException(message, 503)
}

// GatewayTimeoutException 创建 504 Gateway Timeout 异常；message 为空时使用默认文案。
func GatewayTimeoutException(message string) *HttpException {
	if message == "" {
		message = "Gateway Timeout"
	}
	return NewHttpException(message, 504)
}
