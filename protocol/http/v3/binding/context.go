// context.go 定义 HTTP binding 使用的上下文适配层。
package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// Renderer 表示 HTTP adapter 可选提供的模板渲染能力。
type Renderer interface {
	Render(w http.ResponseWriter, r *http.Request, template string, data any) error
}

// Context 是一个接近 gin 风格的 HTTP 上下文抽象。
//
// 它通过 binding 系统注入到 handler 中，默认实现是 `*HttpContext`。
type Context interface {
	Request() *http.Request
	Writer() http.ResponseWriter

	Status(code int)
	Header(key, value string)
	JSON(code int, obj any)
	String(code int, s string)
	Data(code int, contentType string, data []byte)

	Param(key string) string
	Query(key string) string
	QueryDefault(key, defaultValue string) string
	GetHeader(key string) string

	Cookie(name string) string
	Cookies() map[string]string
	SetCookie(cookie *http.Cookie)

	BodyBytes() []byte
	Bind(obj any) error

	// 兼容 gin 风格的命名；内部基于 Bind 实现。
	ShouldBind(obj any) error
	ShouldBindJSON(obj any) error
	ShouldBindQuery(obj any) error
	ShouldBindHeader(obj any) error
	ShouldBindUri(obj any) error
	ShouldBindForm(obj any) error

	File(key string) *UploadedFile
	Files(key string) []*UploadedFile

	Redirect(code int, location string) error
	Render(template string, data any) error
	ServeFile(path string) error
	ServeFileAttachment(path, filename string) error

	Set(key string, value any)
	Get(key string) (any, bool)

	Next() error
	Abort()
	AbortWithError(code int, err error)
	Aborted() bool
}

// HttpContext 是 `binding/http.Context` 的默认实现。
// 它是 runtime.HandlerContext 在 HTTP 语义下的一层薄包装，不持有独立请求生命周期。
type HttpContext struct {
	rctx    *runtime.HandlerContext
	req     *http.Request
	writer  http.ResponseWriter
	aborted bool
}

// NewHttpContext 基于 runtime.HandlerContext 创建一个 HTTP 上下文包装。
func NewHttpContext(rctx *runtime.HandlerContext) (*HttpContext, error) {
	if rctx == nil {
		return nil, fmt.Errorf("nil runtime context")
	}
	req, _ := rctx.Request.Raw.(*http.Request)
	w, _ := rctx.Response.Raw.(http.ResponseWriter)
	if req == nil || w == nil {
		return nil, fmt.Errorf("http raw request/response not set in runtime context")
	}
	return &HttpContext{rctx: rctx, req: req, writer: w}, nil
}

// Request 返回底层 `*http.Request`。
func (c *HttpContext) Request() *http.Request { return c.req }

// Writer 返回底层 `http.ResponseWriter`。
func (c *HttpContext) Writer() http.ResponseWriter { return c.writer }

// Status 设置当前响应状态码（先写到 runtime 响应对象，最终由 adapter 统一落到 ResponseWriter）。
func (c *HttpContext) Status(code int) { c.rctx.Response.Status = code }

// Header 设置响应 Header（写入 runtime.Response.Headers）。
// 最终输出时以 runtime headers 为准，允许 handler 在中途多次覆盖。
func (c *HttpContext) Header(key, value string) { c.rctx.Response.Headers[key] = value }

// JSON 设置 JSON 响应体与 Content-Type。
func (c *HttpContext) JSON(code int, obj any) {
	c.Status(code)
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.rctx.Response.Body = obj
}

// String 设置纯文本响应体与 Content-Type。
func (c *HttpContext) String(code int, s string) {
	c.Status(code)
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.rctx.Response.Body = s
}

// Data 设置二进制响应体，并可选指定 Content-Type。
func (c *HttpContext) Data(code int, contentType string, data []byte) {
	c.Status(code)
	if contentType != "" {
		c.Header("Content-Type", contentType)
	}
	c.rctx.Response.Body = data
}

// Param 读取路径参数。
func (c *HttpContext) Param(key string) string { return c.rctx.Request.Params[key] }

// Query 读取 query 参数。
func (c *HttpContext) Query(key string) string { return c.rctx.Request.Query[key] }

// QueryDefault 读取 query 参数；当为空时返回默认值。
func (c *HttpContext) QueryDefault(key, defaultValue string) string {
	if v := c.Query(key); v != "" {
		return v
	}
	return defaultValue
}

// GetHeader 读取请求 Header。
func (c *HttpContext) GetHeader(key string) string { return c.rctx.Request.Headers[key] }

// Cookie 读取指定名称的 Cookie 值。
func (c *HttpContext) Cookie(name string) string {
	if c.req == nil || name == "" {
		return ""
	}
	if ck, err := c.req.Cookie(name); err == nil && ck != nil {
		return ck.Value
	}
	if v, ok := c.rctx.Get(MetadataKeyCookies); ok {
		if m, ok := v.(map[string]string); ok {
			return m[name]
		}
	}
	return ""
}

// Cookies 读取全部 Cookie（优先使用 binding 预解析缓存）。
func (c *HttpContext) Cookies() map[string]string {
	out := make(map[string]string)
	if v, ok := c.rctx.Get(MetadataKeyCookies); ok {
		if m, ok := v.(map[string]string); ok {
			for k, v := range m {
				out[k] = v
			}
			return out
		}
	}
	if c.req != nil {
		for _, ck := range c.req.Cookies() {
			if ck != nil {
				out[ck.Name] = ck.Value
			}
		}
	}
	return out
}

// SetCookie 向响应写入一个 Cookie。
func (c *HttpContext) SetCookie(cookie *http.Cookie) {
	if c.writer == nil || cookie == nil {
		return
	}
	http.SetCookie(c.writer, cookie)
}

// BodyBytes 返回请求体原始字节。
func (c *HttpContext) BodyBytes() []byte { return c.rctx.Request.BodyBytes }

// Bind 会按 binding/http 的规则把当前请求数据绑定到 obj。
func (c *HttpContext) Bind(obj any) error {
	if err := BindInto(c.rctx, obj); err != nil {
		return err
	}
	if vAny, ok := c.rctx.Get(MetadataKeyValidator); ok && vAny != nil {
		if v, ok := vAny.(Validator); ok && v != nil {
			return v.Validate(obj)
		}
	}
	return nil
}

// ShouldBind 是 Bind 的别名，用于兼容 gin 命名。
func (c *HttpContext) ShouldBind(obj any) error { return c.Bind(obj) }

// ShouldBindJSON 要求请求为 JSON，并将数据绑定到 obj。
func (c *HttpContext) ShouldBindJSON(obj any) error {
	if !isJSONRequest(c.rctx) {
		return fmt.Errorf("request content-type is not json")
	}
	return c.Bind(obj)
}

// ShouldBindQuery 是 Bind 的别名，用于兼容 gin 命名。
func (c *HttpContext) ShouldBindQuery(obj any) error { return c.Bind(obj) }

// ShouldBindHeader 是 Bind 的别名，用于兼容 gin 命名。
func (c *HttpContext) ShouldBindHeader(obj any) error { return c.Bind(obj) }

// ShouldBindUri 是 Bind 的别名，用于兼容 gin 命名。
func (c *HttpContext) ShouldBindUri(obj any) error { return c.Bind(obj) }

// ShouldBindForm 是 Bind 的别名，用于兼容 gin 命名。
func (c *HttpContext) ShouldBindForm(obj any) error { return c.Bind(obj) }

// File 获取单个上传文件（取第一个）。
func (c *HttpContext) File(key string) *UploadedFile { return firstUploadedFile(c.rctx, key) }

// Files 获取指定 key 下的全部上传文件。
func (c *HttpContext) Files(key string) []*UploadedFile { return uploadedFiles(c.rctx, key) }

// Redirect 发送重定向响应，并终止后续处理链。
// 这是直接写底层 writer 的即时行为，因此会标记 `MetadataKeyWritten` 并置 aborted。
func (c *HttpContext) Redirect(code int, location string) error {
	if c.writer == nil || c.req == nil {
		return fmt.Errorf("http context not initialized")
	}
	c.markWritten()
	c.aborted = true
	http.Redirect(c.writer, c.req, location, code)
	return nil
}

// Render 使用 adapter 提供的 Renderer 渲染模板，并终止后续处理链。
// renderer 来自 adapter 在请求入口写入的 metadata。
func (c *HttpContext) Render(template string, data any) error {
	if c.writer == nil || c.req == nil {
		return fmt.Errorf("http context not initialized")
	}
	renderer, ok := c.rctx.Get(MetadataKeyRenderer)
	if !ok || renderer == nil {
		return fmt.Errorf("http renderer not configured")
	}
	r, ok := renderer.(Renderer)
	if !ok || r == nil {
		return fmt.Errorf("invalid http renderer")
	}
	c.markWritten()
	c.aborted = true
	if c.writer.Header().Get("Content-Type") == "" {
		c.writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	return r.Render(c.writer, c.req, template, data)
}

// ServeFile 发送指定路径的静态文件，并终止后续处理链。
func (c *HttpContext) ServeFile(path string) error {
	if c.writer == nil || c.req == nil {
		return fmt.Errorf("http context not initialized")
	}
	c.markWritten()
	c.aborted = true
	http.ServeFile(c.writer, c.req, path)
	return nil
}

// ServeFileAttachment 以附件方式下载指定文件，并终止后续处理链。
func (c *HttpContext) ServeFileAttachment(path, filename string) error {
	if c.writer == nil || c.req == nil {
		return fmt.Errorf("http context not initialized")
	}
	c.markWritten()
	c.aborted = true
	if filename != "" {
		// 这里保持简单处理；调用方可以自行传入安全的 ASCII 文件名。
		c.writer.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	}
	http.ServeFile(c.writer, c.req, path)
	return nil
}

// Set 在 runtime context 上设置一个键值。
func (c *HttpContext) Set(key string, value any) { c.rctx.Set(key, value) }

// Get 从 runtime context 获取一个键值。
func (c *HttpContext) Get(key string) (any, bool) { return c.rctx.Get(key) }

// Next 调用中间件链中的下一个处理器（如果存在）。
func (c *HttpContext) Next() error {
	nextAny, ok := c.rctx.Get(MetadataKeyNext)
	if !ok {
		return nil
	}
	switch fn := nextAny.(type) {
	case func() error:
		return fn()
	case Next:
		return fn()
	default:
		return nil
	}
}

// Abort 中止后续处理链。
func (c *HttpContext) Abort() { c.aborted = true }

// AbortWithError 中止处理链并写入一个简单错误响应体（由 HTTP adapter 序列化）。
// 这里不直接写 writer，而是走 runtime.Response，让统一响应出口保持一致。
func (c *HttpContext) AbortWithError(code int, err error) {
	c.aborted = true
	if code > 0 {
		c.Status(code)
	}
	if err != nil {
		// 让 adapter 把它当作普通响应序列化，而不是 runtime 级错误。
		c.Header("Content-Type", "application/json; charset=utf-8")
		c.rctx.Response.Body = map[string]any{
			"error":  err.Error(),
			"status": code,
		}
	}
}

// Aborted 表示处理链是否已被中止。
func (c *HttpContext) Aborted() bool { return c.aborted }

func (c *HttpContext) markWritten() {
	if c.rctx != nil {
		c.rctx.Set(MetadataKeyWritten, true)
	}
}

// 一组便捷的原始写出辅助方法（可选使用）。
func (c *HttpContext) WriteJSON(code int, obj any) error {
	c.markWritten()
	c.aborted = true
	c.writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.writer.WriteHeader(code)
	return json.NewEncoder(c.writer).Encode(obj)
}
