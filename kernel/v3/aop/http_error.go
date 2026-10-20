// http_error.go 提供完整的 HTTP 语义异常集合，以及把任意 error 归一化为
// HttpException 的助手。
//
// 设计目标：
// - 覆盖常用 4xx / 5xx 状态码，避免业务自己写 magic number
// - 允许携带响应头（405 的 Allow、429 的 Retry-After 等）
// - 保留底层 Cause，支持 errors.Is / errors.As 继续向下匹配
// - 与 kernel 的 Kind 分类对齐，让各协议 adapter 都能复用同一套映射
package aop

import (
	"context"
	stderrors "errors"
	"net/http"
	"strconv"
	"time"

	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
)

// httpStatusText 返回状态码的默认文案。
func httpStatusText(status int) string {
	if text := http.StatusText(status); text != "" {
		return text
	}
	return "HTTP " + strconv.Itoa(status)
}

// NewHttpExceptionWithStatus 创建一个指定状态码的异常。
//
// message 为空时使用该状态码的标准文案。
func NewHttpExceptionWithStatus(status int, message string) *HttpException {
	return statusException(status, message)
}

// statusException 是内部构造入口：message 为空时回退到状态码标准文案。
func statusException(status int, message string) *HttpException {
	if message == "" {
		message = httpStatusText(status)
	}
	return NewHttpException(message, status)
}

// PaymentRequiredException 创建 402 Payment Required 异常。
func PaymentRequiredException(message string) *HttpException {
	return statusException(http.StatusPaymentRequired, message)
}

// MethodNotAllowedException 创建 405 Method Not Allowed 异常。
//
// allow 会写入 `Allow` 响应头；为空时保持不写。
func MethodNotAllowedException(message string, allow ...string) *HttpException {
	e := statusException(http.StatusMethodNotAllowed, message)
	if len(allow) > 0 {
		e.WithHeaders(map[string]string{"Allow": joinHeaderValues(allow)})
	}
	return e
}

// LengthRequiredException 创建 411 Length Required 异常。
func LengthRequiredException(message string) *HttpException {
	return statusException(http.StatusLengthRequired, message)
}

// PreconditionFailedException 创建 412 Precondition Failed 异常。
func PreconditionFailedException(message string) *HttpException {
	return statusException(http.StatusPreconditionFailed, message)
}

// URITooLongException 创建 414 URI Too Long 异常。
func URITooLongException(message string) *HttpException {
	return statusException(http.StatusRequestURITooLong, message)
}

// RangeNotSatisfiableException 创建 416 Range Not Satisfiable 异常。
func RangeNotSatisfiableException(message string) *HttpException {
	return statusException(http.StatusRequestedRangeNotSatisfiable, message)
}

// ExpectationFailedException 创建 417 Expectation Failed 异常。
func ExpectationFailedException(message string) *HttpException {
	return statusException(http.StatusExpectationFailed, message)
}

// LockedException 创建 423 Locked 异常。
func LockedException(message string) *HttpException {
	return statusException(http.StatusLocked, message)
}

// FailedDependencyException 创建 424 Failed Dependency 异常。
func FailedDependencyException(message string) *HttpException {
	return statusException(http.StatusFailedDependency, message)
}

// UpgradeRequiredException 创建 426 Upgrade Required 异常。
func UpgradeRequiredException(message string) *HttpException {
	return statusException(http.StatusUpgradeRequired, message)
}

// PreconditionRequiredException 创建 428 Precondition Required 异常。
func PreconditionRequiredException(message string) *HttpException {
	return statusException(http.StatusPreconditionRequired, message)
}

// TooManyRequestsException 创建 429 Too Many Requests 异常。
//
// retryAfter > 0 时会写入 `Retry-After` 响应头（单位：秒）。
func TooManyRequestsException(message string, retryAfter time.Duration) *HttpException {
	e := statusException(http.StatusTooManyRequests, message)
	if retryAfter > 0 {
		seconds := int(retryAfter.Round(time.Second) / time.Second)
		if seconds < 1 {
			seconds = 1
		}
		e.WithHeaders(map[string]string{"Retry-After": strconv.Itoa(seconds)})
	}
	return e
}

// RequestHeaderFieldsTooLargeException 创建 431 Request Header Fields Too Large 异常。
func RequestHeaderFieldsTooLargeException(message string) *HttpException {
	return statusException(http.StatusRequestHeaderFieldsTooLarge, message)
}

// UnavailableForLegalReasonsException 创建 451 Unavailable For Legal Reasons 异常。
func UnavailableForLegalReasonsException(message string) *HttpException {
	return statusException(http.StatusUnavailableForLegalReasons, message)
}

// HTTPVersionNotSupportedException 创建 505 HTTP Version Not Supported 异常。
func HTTPVersionNotSupportedException(message string) *HttpException {
	return statusException(http.StatusHTTPVersionNotSupported, message)
}

// InsufficientStorageException 创建 507 Insufficient Storage 异常。
func InsufficientStorageException(message string) *HttpException {
	return statusException(http.StatusInsufficientStorage, message)
}

// LoopDetectedException 创建 508 Loop Detected 异常。
func LoopDetectedException(message string) *HttpException {
	return statusException(http.StatusLoopDetected, message)
}

// NetworkAuthenticationRequiredException 创建 511 Network Authentication Required 异常。
func NetworkAuthenticationRequiredException(message string) *HttpException {
	return statusException(http.StatusNetworkAuthenticationRequired, message)
}

// FromError 把任意 error 归一化成 *HttpException。
//
// 识别顺序：
//  1. 已经是 *HttpException（含包装链）
//  2. *ValidationException → 422 + 字段级 Details
//  3. context.DeadlineExceeded → 504，context.Canceled → 499
//  4. *kerrors.KernelError / sentinel：按 Kind 映射
//  5. 其它 → 500
//
// 返回 false 表示 err 为 nil。
func FromError(err error) (*HttpException, bool) {
	if err == nil {
		return nil, false
	}

	var httpErr *HttpException
	if stderrors.As(err, &httpErr) && httpErr != nil {
		if httpErr.Status == 0 {
			httpErr.Status = http.StatusInternalServerError
		}
		return httpErr, true
	}

	var validation *ValidationException
	if stderrors.As(err, &validation) && validation != nil {
		return NewHttpException(validation.Message, http.StatusUnprocessableEntity).
			WithCode("VALIDATION_FAILED").
			WithDetails(validation.Errors).
			WithCause(err), true
	}

	if stderrors.Is(err, context.DeadlineExceeded) {
		return NewHttpException("request timeout", http.StatusGatewayTimeout).WithCause(err), true
	}
	if stderrors.Is(err, context.Canceled) {
		return NewHttpException("request canceled", statusClientClosedRequest).WithCause(err), true
	}

	var kernelErr *kerrors.KernelError
	if stderrors.As(err, &kernelErr) && kernelErr != nil {
		status := StatusFromKind(kernelErr.Kind)
		return NewHttpException(kernelErr.Error(), status).WithCause(err), true
	}

	status := StatusFromKind(kerrors.Classify(err))
	if status == 0 {
		status = http.StatusInternalServerError
	}
	return NewHttpException(httpStatusText(status), status).WithCause(err), true
}

// statusClientClosedRequest 是 nginx 约定俗成的 499（客户端提前断开）。
const statusClientClosedRequest = 499

// StatusFromKind 把 kernel 错误分类映射为 HTTP 状态码。
//
// 返回 0 表示该 Kind 没有固定映射，由调用方决定兜底值。
func StatusFromKind(kind kerrors.Kind) int {
	switch kind {
	case kerrors.KindRouteNotFound:
		return http.StatusNotFound
	case kerrors.KindUnauthorized:
		return http.StatusUnauthorized
	case kerrors.KindForbidden:
		return http.StatusForbidden
	case kerrors.KindValidation:
		return http.StatusUnprocessableEntity
	case kerrors.KindBinding, kerrors.KindDI:
		return http.StatusBadRequest
	case kerrors.KindExecution:
		return http.StatusInternalServerError
	default:
		return 0
	}
}

func joinHeaderValues(values []string) string {
	joined := ""
	for index, value := range values {
		if index > 0 {
			joined += ", "
		}
		joined += value
	}
	return joined
}
