// transport.go 实现 HTTP adapter 的传输层入口与请求分发衔接逻辑。
package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	httpbinding "github.com/sao-lang/lania-g/protocol/http/v3/binding"
)

// ErrorResponse 是 HTTP adapter 的统一错误响应体。
//
// 所有由框架产生的错误（绑定失败、校验失败、Guard 拒绝、业务 HttpException、
// 未预期 panic）都使用这一结构，便于客户端稳定解析。
type ErrorResponse struct {
	// Code 是业务错误码：HttpException 显式指定优先，否则为 `HTTP-<status>`。
	Code string `json:"code"`
	// Message 是人类可读文案。
	Message string `json:"message"`
	// Details 是结构化细节（例如字段级校验错误）。
	Details any `json:"details,omitempty"`
	// Status 与 HTTP 状态码一致，方便只读 body 的客户端。
	Status int `json:"status"`
	// Path 是请求路径。
	Path string `json:"path,omitempty"`
	// Kind 是内核错误分类（Binding / Validation / Forbidden ...）。
	Kind string `json:"kind,omitempty"`
	// RequestID 来自请求上下文（若启用了 RequestID 中间件）。
	RequestID string `json:"requestId,omitempty"`
}

// WriteError 把任意 error 写成框架统一错误响应。
//
// 它复用 adapter 内部同一套归一化逻辑（Kind → 状态码、HttpException 覆盖、
// Headers/Code/Details 透传），供自定义 transport 或中间件复用。
func WriteError(w http.ResponseWriter, r *http.Request, ctx *runtime.HandlerContext, err error) {
	writeHTTPError(w, r, ctx, err)
}

// writeHTTPError 负责把框架内部 error 统一投影成 HTTP 错误响应。
// 这里先把任意 error 归一化成 KernelError，再映射到合适的状态码和 JSON 结构。
func writeHTTPError(w http.ResponseWriter, r *http.Request, ctx *runtime.HandlerContext, err error) {
	moduleKey := ""
	if ctx != nil {
		if value, ok := ctx.Get("kernel.moduleKey"); ok {
			moduleKey, _ = value.(string)
		}
	}
	protocol, routeKey := "", ""
	if ctx != nil {
		protocol = string(ctx.Protocol)
		routeKey = ctx.RouteKey
	}
	kerr := kerrors.Normalize(protocol, routeKey, moduleKey, err)
	writeKernelError(w, r, ctx, kerr)
}

// writeKernelError 把已归一化的 KernelError 写成统一错误响应。
func writeKernelError(w http.ResponseWriter, r *http.Request, ctx *runtime.HandlerContext, kerr *kerrors.KernelError) {
	if kerr == nil {
		return
	}

	httpErr, _ := aop.FromError(kerr)
	status := httpErr.Status
	if status == 0 {
		status = http.StatusInternalServerError
	}

	for key, value := range httpErr.Headers {
		w.Header().Set(key, value)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	response := ErrorResponse{
		Code:      httpErr.Code,
		Message:   httpErr.Message,
		Details:   httpErr.Details,
		Status:    status,
		Path:      r.URL.Path,
		Kind:      string(kerr.Kind),
		RequestID: requestIDOf(ctx, r),
	}
	if response.Code == "" {
		response.Code = "HTTP-" + strconv.Itoa(status)
	}
	_ = json.NewEncoder(w).Encode(response)
}

// requestIDOf 从请求上下文或请求头里取 RequestID，供错误响应回显。
func requestIDOf(ctx *runtime.HandlerContext, r *http.Request) string {
	if ctx != nil {
		if value, ok := ctx.Get("kernel.requestId"); ok {
			if id, ok := value.(string); ok {
				return id
			}
		}
	}
	if r != nil {
		return r.Header.Get("X-Request-Id")
	}
	return ""
}

// writeRequestError 用于尚未构造 HandlerContext 的早期失败（例如 body 超限）。
func writeRequestError(w http.ResponseWriter, r *http.Request, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{
		Code:    "HTTP-" + strconv.Itoa(status),
		Message: message,
		Status:  status,
		Path:    r.URL.Path,
	})
}

// writeHTTPResponse 把 runtime handler 的执行结果写回真实 HTTP 响应。
// 它是 HTTP adapter 的最终统一出口：状态码、响应头、重定向、模板渲染、JSON 序列化都在这里收口。
func writeHTTPResponse(w http.ResponseWriter, r *http.Request, handler *runtime.Handler, ctx *runtime.HandlerContext, result any, renderer httpbinding.Renderer) {
	status := ctx.Response.Status
	if handler != nil && status == 200 && handler.Meta.StatusCode > 0 {
		status = handler.Meta.StatusCode
	}

	// 先应用编译期 handler meta，再应用运行期 ctx.Response，
	// 这样 handler/中间件可以覆盖 DSL 上预先声明的默认 header。
	if handler != nil {
		for k, v := range handler.Meta.Headers {
			w.Header().Set(k, v)
		}
	}
	for k, v := range ctx.Response.Headers {
		w.Header().Set(k, v)
	}

	if handler != nil && handler.Meta.RedirectURL != "" {
		code := handler.Meta.RedirectCode
		if code == 0 {
			code = http.StatusFound
		}
		http.Redirect(w, r, handler.Meta.RedirectURL, code)
		return
	}

	// 业务若显式写了 ctx.Response.Body，则优先使用它，而不是 runtime.Execute 的返回值。
	if result == nil && ctx.Response.Body != nil {
		result = ctx.Response.Body
	}

	if handler != nil && handler.Meta.Render != "" {
		if renderer != nil {
			if err := renderer.Render(w, r, handler.Meta.Render, result); err != nil {
				writeHTTPError(w, r, ctx, err)
			}
			return
		}
		// 未配置 renderer 时退化为纯文本输出，至少保证 Render 路由不会静默丢响应。
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		}
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, result)
		return
	}

	// HEAD 语义只返回 header/status，不写 body。
	if r.Method == http.MethodHead {
		w.WriteHeader(status)
		return
	}

	switch v := result.(type) {
	case nil:
		w.WriteHeader(status)
		return
	case []byte:
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		w.WriteHeader(status)
		_, _ = w.Write(v)
		return
	case string:
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(v))
		return
	default:
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
}
