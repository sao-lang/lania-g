// types.go 定义 GraphQL 协议暴露给 handler 的 binding wrapper 与辅助类型。
package graphql

import (
	stdctx "context"
	"net/http"
	"strings"

	gqlast "github.com/graphql-go/graphql/language/ast"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// 这些包装类型用于把 GraphQL 参数、请求头、父节点等信息显式映射到 resolver 入参。
// 它们的目标是让 resolver 签名直接表达“我依赖哪一类 GraphQL 运行时值”。
type Arg[T any] struct{ Value T }

// ArgValue 与 Arg 等价，保留另一种命名风格。
type ArgValue[T any] struct{ Value T }

// Header 表示一个请求头绑定值。
type Header[T any] struct{ Value T }

// Parent 表示父节点对象绑定值。
type Parent[T any] struct{ Value T }

// Variables / Headers / Extensions 分别对应 GraphQL 请求中的变量、请求头与扩展字段。
// 它们都是命名 map 类型，便于 resolver 在签名里显式区分不同来源的数据。
type Variables map[string]any

// Headers 表示请求头集合。
type Headers map[string][]string

// Extensions 表示 GraphQL 请求中的扩展字段。
type Extensions map[string]any

// SelectionSet 既保留底层 AST，也额外提取一份扁平字段名列表，
// 方便业务层做最常见的“这次请求选了哪些字段”判断。
type SelectionSet struct {
	Raw    *gqlast.SelectionSet
	Fields []string
}

// Root / Info / OperationName / FieldName / RawQuery 表示常见的 GraphQL 运行时元信息。
type Root any

// Info 表示 GraphQL 执行信息对象。
type Info any

// OperationName 表示操作名。
type OperationName string

// FieldName 表示当前字段名。
type FieldName string

// RawQuery 表示原始 GraphQL 查询文本。
type RawQuery string

// IP 表示请求来源 IP。
type IP string

// Host 表示请求 Host。
type Host string

// Method 表示请求方法。
type Method string

// URL 表示请求 URL。
type URL string

// Path 表示请求路径。
type Path string

// Session 表示会话数据。
type Session map[string]any

// Request 是原始 `*http.Request` 的别名。
type Request = *http.Request

// Response 是原始 `http.ResponseWriter` 的别名。
type Response = http.ResponseWriter

// Context 是 GraphQL 请求上下文抽象，类似于 HTTP/WS 场景中的 Context。
// 它可通过 binding 注入，并由 `*GraphQLContext` 实现。
type Context interface {
	stdctx.Context

	Request() *http.Request
	Writer() http.ResponseWriter

	OperationName() string
	Query() string
	Variables() map[string]any
	Headers() map[string][]string
	Extensions() map[string]any

	FieldType() string
	FieldName() string
	Path() []string
	SelectionSet() *gqlast.SelectionSet

	Root() any
	Info() any
	Args() map[string]any
	Session() map[string]any

	// 提供本地 KV 存储，便于 extensions/middlewares 在一次请求中透传数据。
	Set(key string, value any)
	Get(key string) (any, bool)

	// 一组便捷辅助方法。
	Header(key string) string
	Var(key string) (any, bool)
	Arg(key string) (any, bool)
	ShouldBindArgs(obj any) error
}

// GraphQLContext 是 `binding/graphql.Context` 的默认实现。
// 它把 adapter 在请求入口收集到的散落 metadata 收拢成一个稳定上下文对象。
type GraphQLContext struct {
	stdctx.Context

	request   *http.Request
	writer    http.ResponseWriter
	vars      map[string]any
	headers   map[string][]string
	ext       map[string]any
	session   map[string]any
	rawArgs   map[string]any
	root      any
	info      any
	opName    string
	query     string
	fieldTyp  string
	field     string
	selection *gqlast.SelectionSet
	path      []string

	kv map[string]any

	rctx *runtime.HandlerContext
}

// InitContext 初始化 GraphQLContext 的内部字段。
// GraphQL adapter 会在每次字段执行前用它把运行时 metadata 组装成一个可注入的 Context。
func InitContext(
	c *GraphQLContext,
	base stdctx.Context,
	req *http.Request,
	w http.ResponseWriter,
	operationName string,
	rawQuery string,
	fieldType string,
	fieldName string,
	path []string,
	selection *gqlast.SelectionSet,
	root any,
	info any,
	vars map[string]any,
	headers map[string][]string,
	extensions map[string]any,
	session map[string]any,
	args map[string]any,
) {
	if c == nil {
		return
	}
	if base == nil {
		base = stdctx.Background()
	}
	c.Context = base
	c.request = req
	c.writer = w
	c.opName = operationName
	c.query = rawQuery
	c.fieldTyp = fieldType
	c.field = fieldName
	c.path = path
	c.selection = selection
	c.root = root
	c.info = info
	c.vars = vars
	c.headers = headers
	c.ext = extensions
	c.session = session
	c.rawArgs = args
	if c.kv == nil {
		// kv 只保存“当前请求/当前字段执行”内的临时数据，不做跨请求复用。
		c.kv = map[string]any{}
	}
}

// Request 返回原始 `*http.Request`。
func (c *GraphQLContext) Request() *http.Request { return c.request }

// Writer 返回原始 `http.ResponseWriter`。
func (c *GraphQLContext) Writer() http.ResponseWriter { return c.writer }

// OperationName 返回当前 GraphQL 操作名。
func (c *GraphQLContext) OperationName() string { return c.opName }

// Query 返回原始 GraphQL 查询文本。
func (c *GraphQLContext) Query() string { return c.query }

// Variables 返回变量集合。
func (c *GraphQLContext) Variables() map[string]any { return c.vars }

// Headers 返回请求头集合。
func (c *GraphQLContext) Headers() map[string][]string { return c.headers }

// Extensions 返回扩展字段集合。
func (c *GraphQLContext) Extensions() map[string]any { return c.ext }

// FieldType 返回当前字段所属类型。
func (c *GraphQLContext) FieldType() string { return c.fieldTyp }

// FieldName 返回当前字段名。
func (c *GraphQLContext) FieldName() string { return c.field }

// Path 返回当前字段路径。
func (c *GraphQLContext) Path() []string { return c.path }

// SelectionSet 返回当前字段的选择集。
func (c *GraphQLContext) SelectionSet() *gqlast.SelectionSet { return c.selection }

// Root 返回当前执行的 root 值。
func (c *GraphQLContext) Root() any { return c.root }

// Info 返回 GraphQL 执行信息对象。
func (c *GraphQLContext) Info() any { return c.info }

// Args 返回当前字段参数集合。
func (c *GraphQLContext) Args() map[string]any { return c.rawArgs }

// Session 返回会话数据。
func (c *GraphQLContext) Session() map[string]any { return c.session }

// Set 在当前请求上下文内保存一个键值。
func (c *GraphQLContext) Set(key string, value any) {
	if c == nil || key == "" {
		return
	}
	if c.kv == nil {
		c.kv = map[string]any{}
	}
	c.kv[key] = value
}

// Get 读取当前请求上下文内保存的键值。
func (c *GraphQLContext) Get(key string) (any, bool) {
	if c == nil || c.kv == nil || key == "" {
		return nil, false
	}
	v, ok := c.kv[key]
	return v, ok
}

// Header 按名称读取请求头值（大小写不敏感）。
func (c *GraphQLContext) Header(key string) string {
	if c == nil || key == "" {
		return ""
	}
	if c.headers == nil {
		return ""
	}
	if values, ok := c.headers[key]; ok && len(values) > 0 {
		return values[0]
	}
	// 大小写不敏感兜底（HTTP header 天然大小写不敏感）。
	for k, values := range c.headers {
		if len(values) == 0 {
			continue
		}
		if strings.EqualFold(k, key) {
			return values[0]
		}
	}
	return ""
}

// Var 按名称读取 GraphQL 变量。
func (c *GraphQLContext) Var(key string) (any, bool) {
	if c == nil || c.vars == nil || key == "" {
		return nil, false
	}
	v, ok := c.vars[key]
	return v, ok
}

// Arg 按名称读取当前字段参数。
func (c *GraphQLContext) Arg(key string) (any, bool) {
	if c == nil || c.rawArgs == nil || key == "" {
		return nil, false
	}
	v, ok := c.rawArgs[key]
	return v, ok
}

// AttachHandlerContext 将当前 GraphQLContext 关联到一次具体的 HandlerContext。
//
// 该方法主要供 GraphQL adapter 在执行字段前调用，以便 `ShouldBindArgs(...)`
// 能继续复用运行期 metadata（例如自定义 validator）。
func (c *GraphQLContext) AttachHandlerContext(rctx *runtime.HandlerContext) {
	if c == nil {
		return
	}
	c.rctx = rctx
}
