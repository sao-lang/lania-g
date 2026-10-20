// Package enhancers 提供开箱即用的通用 AOP 增强器。
//
// 这一层可以同时依赖 aop 与 runtime：aop 是纯抽象（不能反向依赖 runtime），
// 而这里的中间件需要读取 `*runtime.HandlerContext` / `*runtime.Response`，
// 因此单独成包，避免循环依赖。
//
// 所有增强器都是协议无关的：HTTP / gRPC / GraphQL / WS / MQ / Scheduler
// 共用同一份实现，只需注册成全局 AOP 即可。
package enhancers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// RequestIDMetadataKey 是 RequestID 写入 HandlerContext 的元数据键。
const RequestIDMetadataKey = "kernel.requestId"

// DefaultRequestIDHeader 是默认透传 RequestID 的请求/响应头。
const DefaultRequestIDHeader = "X-Request-Id"

// Recovery 捕获后续链路的 panic，并转换为 500 错误。
//
// 注意：runtime.Pipeline 自身也会 recover panic（转为 ErrExecutionFailed），
// 这个中间件的作用是提供更可控的上报与更友好的错误文案。
func Recovery(reporter func(recovered any)) aop.MiddlewareFunc {
	return func(ctx *aop.ExecutionContext, next func() error) (err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				if reporter != nil {
					reporter(recovered)
				}
				err = aop.InternalServerErrorException(fmt.Sprintf("panic recovered: %v", recovered))
			}
		}()
		return next()
	}
}

// RequestID 透传或生成请求 ID。
//
// 行为：
// - 请求头已有该 ID 时复用（便于链路追踪）
// - 否则用 generator 生成（默认 16 字节随机 hex）
// - 写入 HandlerContext(`kernel.requestId`)、请求头与响应头
func RequestID(header string, generator func() string) aop.MiddlewareFunc {
	if header == "" {
		header = DefaultRequestIDHeader
	}
	if generator == nil {
		generator = randomID
	}
	return func(ctx *aop.ExecutionContext, next func() error) error {
		hc, ok := ctx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return next()
		}

		id := ""
		if hc.Request != nil && hc.Request.Headers != nil {
			id = hc.Request.Headers[header]
		}
		if id == "" {
			id = generator()
			if hc.Request != nil && hc.Request.Headers != nil {
				hc.Request.Headers[header] = id
			}
		}
		hc.Set(RequestIDMetadataKey, id)
		if hc.Response != nil && hc.Response.Headers != nil {
			hc.Response.Headers[header] = id
		}
		return next()
	}
}

// AccessEntry 是一条访问日志的结构化内容。
type AccessEntry struct {
	Protocol  string
	ModuleKey string
	RouteKey  string
	RequestID string
	Method    string
	Path      string
	Status    int
	Duration  time.Duration
	Err       error
}

// AccessLog 记录一次请求的结构化访问日志。
//
// 它只读取统一的 runtime 抽象，因此对 HTTP/gRPC/WS 都适用：
// HTTP 有 Method/Path，其余协议 Method 为空、Path 取 RouteKey。
func AccessLog(logger func(AccessEntry)) aop.MiddlewareFunc {
	return func(ctx *aop.ExecutionContext, next func() error) error {
		start := time.Now()
		err := next()

		if logger == nil {
			return err
		}

		entry := AccessEntry{
			Protocol:  ctx.Protocol,
			ModuleKey: ctx.ModuleKey,
			RouteKey:  ctx.RouteKey,
			Duration:  time.Since(start),
			Err:       err,
		}
		if hc, ok := ctx.HandlerContext.(*runtime.HandlerContext); ok && hc != nil {
			if hc.Request != nil {
				entry.Method = hc.Request.Method
				entry.Path = hc.Request.Path
			}
			if hc.Response != nil {
				entry.Status = hc.Response.Status
			}
			if value, ok := hc.Get(RequestIDMetadataKey); ok {
				entry.RequestID, _ = value.(string)
			}
		}
		if entry.Path == "" {
			entry.Path = ctx.RouteKey
		}
		if entry.Status == 0 {
			entry.Status = 200
		}
		if err != nil {
			httpErr, _ := aop.FromError(err)
			entry.Status = httpErr.Status
		}
		logger(entry)
		return err
	}
}

// Timeout 为当前请求设置超时 context。
//
// 这是协作式超时：handler 必须尊重 `ctx.GetContext()` 才会被打断（Go 无法强杀 goroutine）。
// 超时后返回 504 Gateway Timeout，便于各协议统一映射。
func Timeout(timeout time.Duration) aop.MiddlewareFunc {
	return func(ctx *aop.ExecutionContext, next func() error) error {
		if timeout <= 0 {
			return next()
		}
		hc, ok := ctx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return next()
		}

		_, cancel := hc.WithTimeout(timeout)
		defer cancel()

		err := next()
		if err != nil && errors.Is(hc.GetContext().Err(), context.DeadlineExceeded) {
			return aop.GatewayTimeoutException("request timeout").WithCause(err)
		}
		return err
	}
}

// ErrorBody 是 ErrorEnvelope 的默认响应体。
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// ErrorEnvelope 把任意错误归一化为统一的响应形态并"消费"它。
//
// 它通过写 `*runtime.Response`（Status / Headers / Body）来生效，
// 因此 HTTP、gRPC、GraphQL 等 adapter 都能复用同一份错误形态。
// body 为 nil 时使用默认的 ErrorBody。
func ErrorEnvelope(body func(*aop.HttpException) any) aop.ExceptionFilterFunc {
	return func(exception interface{}, ctx *aop.ExecutionContext) error {
		err, ok := exception.(error)
		if !ok || err == nil {
			return nil
		}
		httpErr, ok := aop.FromError(err)
		if !ok {
			return err
		}

		response, ok := ctx.Response.(*runtime.Response)
		if !ok || response == nil {
			// 没有统一响应抽象时不要吞掉错误，交给 adapter 处理。
			return err
		}

		if response.Headers == nil {
			response.Headers = make(map[string]string)
		}
		for key, value := range httpErr.Headers {
			response.Headers[key] = value
		}
		response.Status = httpErr.Status
		if body != nil {
			response.Body = body(httpErr)
		} else {
			response.Body = ErrorBody{
				Code:    errorCode(httpErr),
				Message: httpErr.Message,
				Details: httpErr.Details,
			}
		}
		return nil
	}
}

func errorCode(httpErr *aop.HttpException) string {
	if httpErr == nil {
		return ""
	}
	if httpErr.Code != "" {
		return httpErr.Code
	}
	return "HTTP-" + fmt.Sprint(httpErr.Status)
}

func randomID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}
