// types.go 定义 HTTP 协议暴露给 handler 的 binding wrapper 与辅助类型。
package http

import (
	"errors"
	"mime/multipart"
	"net/http"
)

// 这些泛型包装类型用于显式声明“参数该从 HTTP 请求的哪个位置取值”。
// 这种“类型即语义”的做法，能让 handler 签名直接表达依赖来源，而不必靠注释猜。
type Body[T any] struct{ Value T }

// Query 表示一个 query 参数绑定值。
type Query[T any] struct{ Value T }

// Param 表示一个路径参数绑定值。
type Param[T any] struct{ Value T }

// Header 表示一个请求 Header 绑定值。
type Header[T any] struct{ Value T }

// Form 表示一个表单字段绑定值。
type Form[T any] struct{ Value T }

// Bind 表示“聚合绑定”：按 struct tag 从多个来源把值绑定到 DTO。
type Bind[T any] struct{ Value T }

// MustBind 表示“聚合绑定”的 panic 版本。
type MustBind[T any] struct{ Value T }

// UploadedFile 是对 `multipart.FileHeader` 的轻量包装。
// 它把标准库上传对象转换成更稳定的框架侧视图，同时保留 `Open()` 能力。
type UploadedFile struct {
	Filename   string
	Size       int64
	Header     map[string][]string
	fileHeader *multipart.FileHeader
}

// File/Files 用于声明单文件或多文件上传参数。
type File struct{ Value *UploadedFile }

// Files 表示多文件上传参数。
type Files struct{ Value []*UploadedFile }

// 这些标量类型直接映射请求中的常见基础信息。
// 使用命名类型而不是直接写 string/map，可以让 resolver 签名更可读。
type IP string

// Host 表示请求 Host。
type Host string

// Method 表示请求方法。
type Method string

// Path 表示请求路径。
type Path string

// URL 表示请求 URL 字符串。
type URL string

// Headers 表示请求头集合。
type Headers map[string][]string

// Session 表示 session 信息（如果 adapter 注入了它）。
type Session map[string]any

// Next 用于在中间件场景中继续后续处理链。
// 只有运行在 adapter middleware 里的 handler 才真正会用到它。
type Next func() error

// BodyBytes / MustBodyBytes 用于直接读取原始请求体字节。
type BodyBytes []byte

// MustBodyBytes 是 BodyBytes 的 panic 版本。
type MustBodyBytes []byte

// BodyAs / MustBodyAs 用于把请求体按指定类型解析为值。
type BodyAs[T any] struct{ Value T }

// MustBodyAs 是 BodyAs 的 panic 版本。
type MustBodyAs[T any] struct{ Value T }

// Original 表示原始 `*http.Request`。
type Original *http.Request

// Cookie / Cookies 用于按名称读取 Cookie 或一次性读取全部 Cookie。
type Cookie[T any] struct{ Value T }

// Cookies 表示全部 Cookie 集合。
type Cookies map[string]string

var errUploadedFileUnavailable = errors.New("uploaded file is unavailable")

// NewUploadedFile 把 `multipart.FileHeader` 包装成支持 `Open()` 的 `UploadedFile`。
func NewUploadedFile(fh *multipart.FileHeader) *UploadedFile {
	if fh == nil {
		return nil
	}
	return &UploadedFile{
		Filename:   fh.Filename,
		Size:       fh.Size,
		Header:     map[string][]string(fh.Header),
		fileHeader: fh,
	}
}

// Open 打开底层上传文件的输入流。
func (f *UploadedFile) Open() (multipart.File, error) {
	if f == nil || f.fileHeader == nil {
		return nil, errUploadedFileUnavailable
	}
	return f.fileHeader.Open()
}

// FileHeader 返回底层原始的 `multipart.FileHeader`，可能为 nil。
func (f *UploadedFile) FileHeader() *multipart.FileHeader {
	if f == nil {
		return nil
	}
	return f.fileHeader
}

// 这些类型用于承载认证集成写入的用户、令牌与权限信息。
// binding/http 自己不做鉴权，只把 guard/interceptor 先写入 metadata 的结果投影出来。
type AuthUser struct{ Value any }

// AuthUserID 表示认证用户 ID。
type AuthUserID struct{ Value string }

// AuthOptionalUser 表示“可能已认证的用户”。
type AuthOptionalUser struct {
	Value         any
	Authenticated bool
}

// AuthToken 表示认证令牌与其解析出的 claims。
type AuthToken struct {
	Value  string
	Claims map[string]any
}

// AuthOptionalToken 表示“可能已认证的 token”。
type AuthOptionalToken struct {
	Value         string
	Claims        map[string]any
	Authenticated bool
}

// AuthRole / AuthPermission 是权限系统的占位类型，便于在 handler 参数中声明依赖。
type AuthRole struct{}

// AuthPermission 表示权限声明占位类型。
type AuthPermission struct{}
