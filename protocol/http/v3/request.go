// request.go 提供 HTTP adapter 的请求包装与上下文辅助。
package http

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	httpbinding "github.com/sao-lang/lania-g/protocol/http/v3/binding"
)

var errRequestBodyTooLarge = errors.New("request body too large")

// readBodyLimited 在入口一次性读取请求体，并把 body 重置成可再次读取的 reader。
// 这样 binding 层既能消费 `BodyBytes`，也不会破坏底层 `http.Request.Body` 的可读性预期。
func readBodyLimited(r *http.Request, max int64) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	limited := io.LimitReader(r.Body, max+1)
	b, err := io.ReadAll(limited)
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, errRequestBodyTooLarge
	}
	return b, nil
}

// normalizeBasePath 用于处理 adapter 挂载根路径；根路径最终统一记为空串。
func normalizeBasePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || p == "/" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = strings.TrimSuffix(p, "/")
	return p
}

// normalizePrefix 用于 mounted 子 handler 的匹配前缀；这里保留 `/` 作为显式根前缀。
func normalizePrefix(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if p != "/" {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

func (a *Adapter) matchMounted(path string) http.Handler {
	path = normalizePrefix(path)
	a.mountMu.RLock()
	defer a.mountMu.RUnlock()
	for _, m := range a.mounted {
		prefix := normalizePrefix(m.pattern)
		// mounted handler 使用“前缀命中”语义，和大多数 HTTP router 的子应用挂载行为保持一致。
		if prefix == "/" || path == prefix || strings.HasPrefix(path, prefix+"/") {
			return m.handler
		}
	}
	return nil
}

// cloneRequestWithPath 复制请求并替换路径，供 mounted 子 handler 在“去掉挂载前缀后”的路径视角运行。
func cloneRequestWithPath(r *http.Request, path string) *http.Request {
	if r == nil || r.URL == nil {
		return r
	}
	r2 := r.Clone(r.Context())
	u2 := *r.URL
	u2.Path = path
	u2.RawPath = ""
	r2.URL = &u2
	return r2
}

// queryValues 同时构造单值与多值视图：
// - single 取第一个值，适合绝大多数简单绑定
// - multi 保留全部值，供 `Headers` / 更细粒度解析使用
func queryValues(values map[string][]string) (single map[string]string, multi map[string][]string) {
	single = make(map[string]string, len(values))
	multi = make(map[string][]string, len(values))
	for k, v := range values {
		if len(v) > 0 {
			single[k] = v[0]
			multi[k] = append([]string{}, v...)
		} else {
			multi[k] = nil
		}
	}
	return single, multi
}

// copyHeaderValues 把 net/http header 同步到 runtime.Request 的单值/多值双视图。
func copyHeaderValues(dstSingle map[string]string, dstMulti map[string][]string, hdr http.Header) {
	for k := range dstSingle {
		delete(dstSingle, k)
	}
	for k := range dstMulti {
		delete(dstMulti, k)
	}
	for k, v := range hdr {
		if len(v) > 0 {
			dstSingle[k] = v[0]
			dstMulti[k] = append([]string{}, v...)
		} else {
			dstMulti[k] = nil
		}
	}
}

// shouldReadBody 尽量避免对明显“无 body”请求做无意义读取，
// 但对 JSON / form / chunked 请求仍保持积极读取，便于后续 binding 统一消费。
func shouldReadBody(r *http.Request) bool {
	if r == nil || r.Body == nil {
		return false
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return false
	}
	if r.ContentLength > 0 {
		return true
	}
	if strings.TrimSpace(r.Header.Get("Transfer-Encoding")) != "" {
		return true
	}
	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.HasPrefix(ct, "application/json") || strings.Contains(ct, "+json") {
		return true
	}
	if strings.HasPrefix(ct, "application/x-www-form-urlencoded") || strings.HasPrefix(ct, "multipart/form-data") {
		return true
	}
	return false
}

// parseUploadedFiles 只在 multipart/form-data 请求上工作，
// 并把标准库的 FileHeader 转成 binding/http 的 UploadedFile 视图。
func parseUploadedFiles(r *http.Request) map[string][]*httpbinding.UploadedFile {
	if r == nil {
		return nil
	}
	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if !strings.HasPrefix(ct, "multipart/form-data") {
		return nil
	}
	if r.MultipartForm == nil {
		_ = r.ParseMultipartForm(32 << 20)
	}
	if r.MultipartForm == nil || r.MultipartForm.File == nil {
		return nil
	}
	out := make(map[string][]*httpbinding.UploadedFile)
	for field, fhs := range r.MultipartForm.File {
		list := make([]*httpbinding.UploadedFile, 0, len(fhs))
		for _, fh := range fhs {
			if uf := httpbinding.NewUploadedFile(fh); uf != nil {
				list = append(list, uf)
			}
		}
		out[field] = list
	}
	return out
}

// parseFormValues 统一抽取 multipart form 与 x-www-form-urlencoded 的表单字段。
func parseFormValues(r *http.Request) map[string][]string {
	if r == nil {
		return nil
	}
	ct := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if r.MultipartForm != nil && len(r.MultipartForm.Value) > 0 {
		out := make(map[string][]string, len(r.MultipartForm.Value))
		for k, v := range r.MultipartForm.Value {
			out[k] = append([]string{}, v...)
		}
		return out
	}
	if !strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
		return nil
	}
	_ = r.ParseForm()
	if len(r.PostForm) == 0 {
		return nil
	}
	out := make(map[string][]string, len(r.PostForm))
	for k, v := range r.PostForm {
		out[k] = append([]string{}, v...)
	}
	return out
}
