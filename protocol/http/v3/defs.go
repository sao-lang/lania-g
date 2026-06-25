// defs.go 定义 HTTP adapter 在 registry 与编译阶段使用的声明结构。
package http

// Method 表示 HTTP 方法。
type Method string

const (
	// GET 表示 HTTP GET 方法。
	GET     Method = "GET"
	// POST 表示 HTTP POST 方法。
	POST    Method = "POST"
	// PUT 表示 HTTP PUT 方法。
	PUT     Method = "PUT"
	// DELETE 表示 HTTP DELETE 方法。
	DELETE  Method = "DELETE"
	// PATCH 表示 HTTP PATCH 方法。
	PATCH   Method = "PATCH"
	// HEAD 表示 HTTP HEAD 方法。
	HEAD    Method = "HEAD"
	// OPTIONS 表示 HTTP OPTIONS 方法。
	OPTIONS Method = "OPTIONS"
	// ALL 表示匹配所有 HTTP 方法。
	ALL     Method = "ALL"
)

// RedirectConfig 描述路由命中的重定向响应配置。
type RedirectConfig struct {
	URL    string
	Status int
}

// SecurityRequirement 描述接口文档中的一条安全要求。
type SecurityRequirement struct {
	Name   string
	Scopes []string
}

// RouteDoc 描述一条 HTTP 路由的文档元信息。
// 这些字段只用于文档生成/描述层，不直接参与运行时匹配和执行。
type RouteDoc struct {
	Hidden         bool
	Summary        string
	Description    string
	Tags           []string
	Security       []SecurityRequirement
	ResponseType   any
	ResponseField  string
	ErrorResponses map[int]string
}

// RouteDefinition 表示一条 HTTP 路由的编译期声明。
// 它由 DSL 收集、由 plugin 编译，最后变成 runtime.Handler。
type RouteDefinition struct {
	Method     Method
	Path       string
	Controller any
	MethodName string

	Guards       []any
	Interceptors []any
	Middlewares  []any
	Pipes        []any
	ParamPipes   map[int][]any
	Filters      []any

	// 下面这些字段是 HTTP 特有的响应语义配置：
	// 状态码、固定响应头、重定向、模板渲染以及文档信息。
	StatusCode int
	Headers    map[string]string
	Redirect   *RedirectConfig
	Render     string
	Doc        *RouteDoc
}
