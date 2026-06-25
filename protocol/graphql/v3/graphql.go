// graphql.go 实现 GraphQL adapter 的主入口与宿主集成逻辑。
package graphql

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	gqlbinding "github.com/sao-lang/lania-g/protocol/graphql/v3/binding"

	gqlast "github.com/graphql-go/graphql/language/ast"
)

// GraphQLRequest 表示一个 GraphQL HTTP 请求体。
type GraphQLRequest struct {
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables"`
	OperationName string         `json:"operationName"`
	Extensions    map[string]any `json:"extensions"`
}

// GraphQLResponse 表示 GraphQL HTTP 接口的响应结构。
type GraphQLResponse struct {
	Data       any   `json:"data,omitempty"`
	Errors     []any `json:"errors,omitempty"`
	Extensions any   `json:"extensions,omitempty"`
}

// GraphQLError 表示 GraphQL 错误结构（可用于返回给客户端）。
type GraphQLError struct {
	Message    string         `json:"message"`
	Path       []string       `json:"path,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

// Error 实现 error 接口，返回错误消息文本。
func (e *GraphQLError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// OperationContext 表示一次 GraphQL 操作执行过程中的上下文信息集合。
type OperationContext struct {
	Context      context.Context
	HTTPRequest  *http.Request
	HTTPResponse http.ResponseWriter
	Request      *GraphQLRequest
	Operation    *gqlast.OperationDefinition
	Schema       *compiledSchema
	Fragments    map[string]*gqlast.FragmentDefinition
	Variables    map[string]any
	Response     *GraphQLResponse
	Errors       []*GraphQLError
}

// ExecutionInfo 描述一次字段解析时的上下文信息。
type ExecutionInfo struct {
	Field         *gqlast.Field
	ParentType    string
	ReturnType    string
	Path          []string
	OperationName string
}

// Adapter 是 GraphQL 协议的运行期适配器，负责挂载 HTTP handler 并把请求分发到 runtime.Execute。
type Adapter struct {
	api *API

	host   adapter.Host
	path   string
	addr   string
	server *http.Server

	playgroundEnabled bool
	playgroundPath    string
	maxBodyBytes      int64
	errorFormatter    func(error) any
	contextFactory    func(*http.Request) *gqlbinding.GraphQLContext
	validator         gqlbinding.Validator
	localSchema       *Schema

	mountedToHTTP bool
	listener      net.Listener
	standalone    bool
	schema        *compiledSchema
	mu            sync.RWMutex
}

var _ adapter.Adapter = (*Adapter)(nil)

// New 创建 GraphQL adapter。
// - `New()` 使用共享监听模式：挂到共享 HTTP listener 上，需要 HTTP adapter + `app.Listen(...)`
// - `New(":8081")` 使用独立监听模式：adapter 自己启动一个 HTTP 服务
//
// 这是该 adapter 唯一的公开端口绑定入口。
func New(addr ...string) *Adapter {
	a := &Adapter{
		api:               NewCompatAPI(),
		path:              "/graphql",
		addr:              "",
		playgroundPath:    "/playground",
		maxBodyBytes:      4 << 20,
		playgroundEnabled: false,
	}
	if len(addr) > 0 && strings.TrimSpace(addr[0]) != "" {
		a.standalone = true
		a.addr = addr[0]
	}
	return a
}

// Listen 仅为兼容旧写法保留。
// Deprecated: 请改用 `graphql.New(":addr")`。
func Listen(addr string) *Adapter { return New(addr) }

// ID 返回 adapter ID。
func (a *Adapter) ID() string { return AdapterID }

// API 返回该协议的 DSL 入口（用于声明 resolver/schema 等）。
func (a *Adapter) API() any { return a.api }

// Plugins 返回该协议需要安装的编译插件。
func (a *Adapter) Plugins() []compiler.ProtocolPlugin {
	return []compiler.ProtocolPlugin{NewPlugin()}
}

// WithPath 设置 GraphQL HTTP 处理路径（默认 `/graphql`）。
func (a *Adapter) WithPath(path string) *Adapter {
	if path == "" {
		return a
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	a.path = path
	return a
}

// WithAddr 仅为兼容旧写法保留。
// Deprecated: 请改用 `graphql.New(":addr")`。
func (a *Adapter) WithAddr(addr string) *Adapter {
	a.standalone = true
	if addr != "" {
		a.addr = addr
	}
	return a
}

// AsStandalone 仅为兼容旧写法保留。
// Deprecated: 请改用 `graphql.New(":addr")`。
func (a *Adapter) AsStandalone(addr string) *Adapter {
	a.standalone = true
	if addr != "" {
		a.addr = addr
	}
	return a
}

// WithSchema 为当前 adapter 指定一份本地 schema。
// 如果设置了它，启动和刷新时会优先使用这份 schema。
func (a *Adapter) WithSchema(schema *Schema) *Adapter {
	a.localSchema = schema
	return a
}

// WithPlayground 控制是否启用 GraphQL Playground。
func (a *Adapter) WithPlayground(enabled bool) *Adapter {
	a.playgroundEnabled = enabled
	return a
}

// MountPlayground 启用 Playground，并可选指定挂载路径。
func (a *Adapter) MountPlayground(path string) *Adapter {
	a.playgroundEnabled = true
	if path != "" {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		a.playgroundPath = path
	}
	return a
}

// WithMaxBodyBytes 设置 GraphQL HTTP 请求体的最大字节数。
func (a *Adapter) WithMaxBodyBytes(n int64) *Adapter {
	if n > 0 {
		a.maxBodyBytes = n
	}
	return a
}

// WithErrorFormatter 设置错误格式化函数，用于把内部 error 转换成响应里的错误结构。
func (a *Adapter) WithErrorFormatter(fn func(error) any) *Adapter {
	a.errorFormatter = fn
	return a
}

// WithValidator 注入 GraphQL 参数 DTO 校验器，供 `binding/graphql.Context.ShouldBindArgs(...)` 使用。
func (a *Adapter) WithValidator(v gqlbinding.Validator) *Adapter {
	a.validator = v
	return a
}

// WithValidatorV10 把 `github.com/go-playground/validator/v10` 配置为默认校验器。
func (a *Adapter) WithValidatorV10() *Adapter {
	return a.WithValidator(gqlbinding.NewValidatorV10())
}

// WithContextFactory 设置 GraphQLContext 构造函数。
// 适合在每次请求进入时注入用户、会话、租户等上下文信息。
func (a *Adapter) WithContextFactory(fn func(*http.Request) *gqlbinding.GraphQLContext) *Adapter {
	a.contextFactory = fn
	return a
}

// Mount 将 GraphQL adapter 挂到应用 host 上，并尝试挂载到共享 HTTP adapter。
func (a *Adapter) Mount(host adapter.Host) error {
	a.host = host
	a.api = NewAPI(host.Registry())
	if err := a.refreshSchema(); err != nil {
		return err
	}
	a.mountedToHTTP = a.tryMountToSharedHTTP(host) == nil
	return nil
}

// Start 启动 GraphQL adapter。
//
// 如果是独立监听模式，会自行启动一个 HTTP 服务；
// 否则会要求自己已经成功挂载到共享 HTTP adapter 上。
func (a *Adapter) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.host == nil {
		return fmt.Errorf("graphql adapter not mounted")
	}
	if err := a.refreshSchemaLocked(); err != nil {
		return err
	}
	if a.standalone {
		if strings.TrimSpace(a.addr) == "" {
			return fmt.Errorf("graphql standalone adapter requires addr")
		}
		if a.server != nil || a.listener != nil {
			return nil
		}
		return a.startStandaloneLocked()
	}
	if err := a.tryMountToSharedHTTP(a.host); err == nil {
		a.mountedToHTTP = true
	}
	if !a.mountedToHTTP {
		return fmt.Errorf("graphql adapter not mounted to http: mount http adapter or use graphql.New(\":addr\")")
	}
	return nil
}

// Stop 停止 GraphQL adapter，并关闭内部持有的 server/listener。
func (a *Adapter) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.server != nil {
		_ = a.server.Close()
		a.server = nil
	}
	if a.listener != nil {
		_ = a.listener.Close()
		a.listener = nil
	}
	a.mountedToHTTP = false
	return nil
}

// Handler 返回当前 adapter 对外暴露的 `http.Handler`。
func (a *Adapter) Handler() http.Handler { return a }

func (a *Adapter) startStandaloneLocked() error {
	ln, err := net.Listen("tcp", a.addr)
	if err != nil {
		return err
	}
	a.listener = ln
	a.server = &http.Server{Addr: a.addr, Handler: a}
	go func() { _ = a.server.Serve(ln) }()
	return nil
}

// listenBlocking 保留一个阻塞式的 listen 实现（用于调试/兼容场景）。
//
//lint:ignore U1000 目前未被引用，但保留该实现以便后续需要阻塞启动模式时直接复用。
func (a *Adapter) listenBlocking() error {
	a.mu.Lock()
	if a.server != nil || a.listener != nil {
		a.mu.Unlock()
		return nil
	}
	ln, err := net.Listen("tcp", a.addr)
	if err != nil {
		a.mu.Unlock()
		return err
	}
	a.listener = ln
	a.server = &http.Server{Addr: a.addr, Handler: a}
	server := a.server
	a.mu.Unlock()
	return server.Serve(ln)
}

func (a *Adapter) tryMountToSharedHTTP(host adapter.Host) error {
	if host == nil {
		return fmt.Errorf("host is nil")
	}
	mh, ok := host.(adapter.HTTPMountHost)
	if !ok {
		return fmt.Errorf("host does not support http mount")
	}
	if err := mh.MountHTTP(a.path, a); err != nil {
		return err
	}
	if a.playgroundEnabled {
		if err := mh.MountHTTP(a.playgroundPath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			a.servePlayground(w)
		})); err != nil {
			return err
		}
	}
	return nil
}

func (a *Adapter) ensureSchema() error {
	a.mu.RLock()
	ok := a.schema != nil
	a.mu.RUnlock()
	if ok {
		return nil
	}
	return a.refreshSchema()
}

func (a *Adapter) refreshSchema() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.refreshSchemaLocked()
}

func (a *Adapter) refreshSchemaLocked() error {
	if a.host == nil {
		return nil
	}
	schema, err := buildCompiledSchemaFromRegistry(a.host.Registry(), a.localSchema)
	if err != nil {
		return err
	}
	a.schema = schema
	return nil
}

func (a *Adapter) schemaSnapshot() *compiledSchema {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.schema
}
