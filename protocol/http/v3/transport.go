// transport.go 实现 HTTP adapter 的传输层入口与请求分发衔接逻辑。
package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	httpbinding "github.com/sao-lang/lania-g/protocol/http/v3/binding"
	"github.com/sao-lang/lania-g/kernel/v3/aop"
	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// writeHTTPError 负责把框架内部 error 统一投影成 HTTP 错误响应。
// 这里先把任意 error 归一化成 KernelError，再映射到合适的状态码和 JSON 结构。
func writeHTTPError(w http.ResponseWriter, r *http.Request, ctx *runtime.HandlerContext, err error) {
	moduleKey := ""
	if ctx != nil {
		if value, ok := ctx.Get("kernel.moduleKey"); ok {
			moduleKey, _ = value.(string)
		}
	}
	kerr := kerrors.Normalize(string(ctx.Protocol), ctx.RouteKey, moduleKey, err)
	if kerr == nil {
		return
	}

	status := http.StatusInternalServerError
	switch {
	case kerr.Kind == kerrors.KindRouteNotFound:
		status = http.StatusNotFound
	case kerr.Kind == kerrors.KindForbidden:
		status = http.StatusForbidden
	case kerr.Kind == kerrors.KindBinding,
		kerr.Kind == kerrors.KindValidation,
		kerr.Kind == kerrors.KindDI:
		status = http.StatusBadRequest
	}

	var httpErr *aop.HttpException
	if errors.As(kerr, &httpErr) && httpErr != nil && httpErr.Status != 0 {
		// 若业务主动抛的是 HTTP 语义异常，则以异常内的状态码优先。
		status = httpErr.Status
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":  kerr.Error(),
		"status": status,
		"path":   r.URL.Path,
		"kind":   kerr.Kind,
		"meta":   kerr.Meta,
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
