// http.go 实现 HTTP adapter 的主入口与宿主集成逻辑。
package http

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	httpbinding "github.com/sao-lang/lania-g/protocol/http/v3/binding"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

const (
	// AdapterID 是 HTTP 协议插件与 adapter 的统一标识。
	AdapterID = "http"
)

// API 是 HTTP adapter 对 registry 的轻量封装入口。
// 它只负责“把声明写入哪里”和“是否支持 mount 一个原生 http.Handler”，不直接承接请求。
type API struct {
	reg            *registry.Registry
	mountHandler   func(prefix string, handler http.Handler)
	fallbackSource string
}

// NewAPI 创建一个 HTTP DSL 入口。
//
// 推荐：使用挂载到应用实例后的 adapter API，或显式传入实例级 registry。
// 兼容：历史上允许 `NewAPI(nil, ...)`，当前等价于 `NewCompatAPI(...)`。
func NewAPI(reg *registry.Registry, mountHandler func(prefix string, handler http.Handler)) *API {
	if reg == nil {
		return NewCompatAPI(mountHandler)
	}
	return &API{reg: reg, mountHandler: mountHandler}
}

// NewCompatAPI 创建一个显式保留给迁移场景的全局 DSL 入口，不作为新代码默认入口。
func NewCompatAPI(mountHandler func(prefix string, handler http.Handler)) *API {
	return globalCompatAPI("http.NewCompatAPI()", mountHandler)
}

func globalCompatAPI(source string, mountHandler func(prefix string, handler http.Handler)) *API {
	return &API{reg: registry.Global(), mountHandler: mountHandler, fallbackSource: source}
}

// Controller 创建一个 controller 级路由声明构建器。
func (api *API) Controller(prefix string, controller any) *ControllerBuilder {
	return newControllerBuilder(prefix, controller, api.reg, api.fallbackSource)
}

// MountHandler 把一个 `net/http` handler 挂载到给定前缀下。
func (api *API) MountHandler(prefix string, handler http.Handler) {
	if api.mountHandler == nil {
		return
	}
	api.mountHandler(prefix, handler)
}

// Adapter 是框架的 HTTP 协议适配器。
// 除了作为 `http.Handler` 对外服务，它还承担挂载子 handler、注入 renderer/validator、
// 以及共享监听/独立监听两种启动模式的协调。
type Adapter struct {
	api          *API
	host         adapter.Host
	addr         string
	standalone   bool
	basePath     string
	server       *http.Server
	nextHandler  http.Handler
	maxBodyBytes int64
	renderer     httpbinding.Renderer
	mu           sync.Mutex
	mountMu      sync.RWMutex
	mounted      []mountedHandler
	cors         *CorsConfig
	helmet       *HelmetConfig
	middlewares  []func(*httpbinding.HttpContext) error
	validator    httpbinding.Validator
}

var _ adapter.Adapter = (*Adapter)(nil)

// New 创建一个 HTTP adapter。
//
// - `New()` 使用 shared listen 模式，地址由 `app.Listen(addr)` 提供
// - `New(":8080")` 使用 standalone 模式，由 adapter 自行监听端口
func New(addr ...string) *Adapter {
	a := &Adapter{
		api:          NewCompatAPI(nil),
		addr:         "",
		basePath:     "",
		nextHandler:  http.NotFoundHandler(),
		maxBodyBytes: 10 << 20,
		mounted:      make([]mountedHandler, 0),
		middlewares:  make([]func(*httpbinding.HttpContext) error, 0),
	}
	if len(addr) > 0 && strings.TrimSpace(addr[0]) != "" {
		a.standalone = true
		a.addr = addr[0]
	}
	return a
}

// Listen 保留用于向后兼容。
// Deprecated: use http.New(":addr") instead.
func Listen(addr string) *Adapter { return New(addr) }

// ID 返回 adapter 标识。
func (a *Adapter) ID() string { return AdapterID }

// API 返回当前 adapter 暴露给应用层使用的 DSL 入口。
func (a *Adapter) API() any { return a.api }

// Plugins 返回 HTTP 协议编译插件集合。
func (a *Adapter) Plugins() []compiler.ProtocolPlugin {
	return []compiler.ProtocolPlugin{NewPlugin()}
}

// Mount 把 adapter 挂到宿主应用上，并接入 registry 与 handler 挂载能力。
func (a *Adapter) Mount(host adapter.Host) error {
	a.host = host
	a.api = NewAPI(host.Registry(), func(prefix string, handler http.Handler) {
		a.MountHandler(prefix, handler)
	})
	return nil
}

// Start 启动 HTTP 服务。
// shared-listen 模式下，addr 由应用通过 `ConfigureSharedListen` 预先注入；
// standalone 模式下，addr 则来自 `http.New(":addr")`。
func (a *Adapter) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.host == nil {
		return fmt.Errorf("http adapter not mounted")
	}
	if strings.TrimSpace(a.addr) == "" {
		if a.standalone {
			return fmt.Errorf("http standalone adapter requires addr")
		}
		return fmt.Errorf("http adapter requires app.Listen(addr) or http.New(\":addr\")")
	}
	if a.server == nil {
		a.server = &http.Server{
			Addr:              a.addr,
			Handler:           a,
			ReadHeaderTimeout: 5 * time.Second,
		}
	}

	ln, err := net.Listen("tcp", a.server.Addr)
	if err != nil {
		return err
	}
	go func() {
		_ = a.server.Serve(ln)
	}()
	return nil
}

// Stop 优雅关闭 HTTP 服务。
func (a *Adapter) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return a.server.Shutdown(ctx)
}

// WithAddr 保留用于向后兼容。
// Deprecated: use http.New(":addr") instead.
func (a *Adapter) WithAddr(addr string) *Adapter {
	a.standalone = true
	a.addr = addr
	return a
}

// AsStandalone 保留用于向后兼容。
// Deprecated: use http.New(":addr") instead.
func (a *Adapter) AsStandalone(addr string) *Adapter {
	a.standalone = true
	a.addr = addr
	return a
}

// Listen 保留用于向后兼容。
// Deprecated: use app.Run() with http.New(":addr") or app.Listen(":addr") with http.New().
func (a *Adapter) Listen(addr string) error {
	a.AsStandalone(addr)
	return a.Start()
}

// WithBasePath 为 adapter 设置统一的路由前缀。
func (a *Adapter) WithBasePath(prefix string) *Adapter {
	a.basePath = prefix
	return a
}

// SetGlobalPrefix 保留 v2 兼容命名，在当前 adapter 中等价于设置 basePath。
func (a *Adapter) SetGlobalPrefix(prefix string) {
	a.WithBasePath(prefix)
}

// RequiresSharedListen 返回当前 adapter 是否依赖应用层提供共享监听地址。
func (a *Adapter) RequiresSharedListen() bool {
	return !a.standalone
}

// ConfigureSharedListen 为 shared listen 模式设置监听地址。
func (a *Adapter) ConfigureSharedListen(addr string) error {
	if a.standalone {
		return nil
	}
	if strings.TrimSpace(addr) == "" {
		return fmt.Errorf("shared listen addr is empty")
	}
	a.addr = addr
	return nil
}

// EnableCors 启用内建 CORS 处理（`net/http` 层）。
func (a *Adapter) EnableCors(config *CorsConfig) *Adapter {
	if config == nil {
		config = DefaultCorsConfig()
	}
	a.cors = config
	return a
}

// EnableHelmet 启用内建安全响应头（`net/http` 层）。
func (a *Adapter) EnableHelmet(config *HelmetConfig) *Adapter {
	if config == nil {
		config = DefaultHelmetConfig()
	}
	a.helmet = config
	return a
}

type mountedHandler struct {
	pattern string
	handler http.Handler
}

// MountHandler 把一个原生 `net/http` handler 挂在某个前缀下。
// 这条链路通常用于静态资源、第三方回调、观测桥等“无需走 runtime.Execute”的能力。
func (a *Adapter) MountHandler(pattern string, handler http.Handler) *Adapter {
	if handler == nil {
		return a
	}
	pattern = normalizePrefix(pattern)
	a.mountMu.Lock()
	defer a.mountMu.Unlock()
	a.mounted = append(a.mounted, mountedHandler{pattern: pattern, handler: handler})
	return a
}

// ServeStatic 把本地目录作为静态资源挂到指定前缀下。
func (a *Adapter) ServeStatic(prefix, root string) *Adapter {
	prefix = normalizePrefix(prefix)
	fs := http.FileServer(http.Dir(root))
	strip := prefix
	if prefix != "/" && !strings.HasSuffix(prefix, "/") {
		strip = prefix + "/"
	}
	return a.MountHandler(prefix, http.StripPrefix(strip, fs))
}

// WithNextHandler 设置路由未命中时的兜底 handler。
func (a *Adapter) WithNextHandler(h http.Handler) *Adapter {
	if h != nil {
		a.nextHandler = h
	}
	return a
}

// WithMaxBodyBytes 设置请求 body 的最大字节数限制。
func (a *Adapter) WithMaxBodyBytes(n int64) *Adapter {
	if n > 0 {
		a.maxBodyBytes = n
	}
	return a
}

// WithRenderer 注入可选的模板渲染器，供 `Render(...)` 等能力使用。
func (a *Adapter) WithRenderer(r httpbinding.Renderer) *Adapter {
	a.renderer = r
	return a
}

// WithValidator 注入参数校验器，供 binding 的 `validate` 阶段使用。
func (a *Adapter) WithValidator(v httpbinding.Validator) *Adapter {
	a.validator = v
	return a
}

// WithValidatorV10 把 `github.com/go-playground/validator/v10` 配置为默认校验器。
// 它会校验 `validate:"..."` 标签；若需兼容 gin 风格必填，可使用 `binding:"required"`。
func (a *Adapter) WithValidatorV10() *Adapter {
	return a.WithValidator(httpbinding.NewValidatorV10())
}

// UseAfter 注册 HTTP middleware。
// 这些 middleware 运行在 adapter 层，围绕 runtime.Execute 形成一条类 gin 的 `ctx.Next()` 链。
func (a *Adapter) UseAfter(mws ...func(*httpbinding.HttpContext) error) *Adapter {
	a.middlewares = append(a.middlewares, mws...)
	return a
}
