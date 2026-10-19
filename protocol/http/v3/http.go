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
)

const (
	// AdapterID 是 HTTP 协议插件与 adapter 的统一标识。
	AdapterID = "http"
)

// API 已移除：路由声明请使用 Controller + Routes。
//
// Adapter 是框架的 HTTP 协议适配器。
// 除了作为 `http.Handler` 对外服务，它还承担挂载子 handler、注入 renderer/validator、
// 以及启动/停止监听等职责。
type Adapter struct {
	host         adapter.Host
	addr         string
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

// newAdapter 根据配置构造适配器。
func newAdapter(cfg Config) *Adapter {
	a := &Adapter{
		addr:         strings.TrimSpace(cfg.Addr),
		basePath:     cfg.BasePath,
		nextHandler:  http.NotFoundHandler(),
		maxBodyBytes: 10 << 20,
		mounted:      make([]mountedHandler, 0),
		middlewares:  make([]func(*httpbinding.HttpContext) error, 0),
	}
	if cfg.MaxBodyBytes > 0 {
		a.maxBodyBytes = cfg.MaxBodyBytes
	}
	if cfg.Renderer != nil {
		a.renderer = cfg.Renderer
	}
	if cfg.ValidatorImpl != nil {
		a.validator = cfg.ValidatorImpl
	} else if cfg.Validator {
		a.validator = httpbinding.NewValidatorV10()
	}
	if cfg.NotFound != nil {
		a.nextHandler = cfg.NotFound
	}
	if cfg.CORS != nil {
		a.cors = cfg.CORS
	}
	if cfg.Helmet != nil {
		a.helmet = cfg.Helmet
	}
	a.middlewares = append(a.middlewares, cfg.Middlewares...)
	return a
}

// ID 返回 adapter 标识。
func (a *Adapter) ID() string { return AdapterID }

// Plugins 返回 HTTP 协议编译插件集合。
func (a *Adapter) Plugins() []compiler.ProtocolPlugin {
	return []compiler.ProtocolPlugin{NewPlugin()}
}

// Mount 把 adapter 挂到宿主应用上，并记录宿主引用。
func (a *Adapter) Mount(host adapter.Host) error {
	a.host = host
	return nil
}

// Start 启动 HTTP 服务，监听地址来自 Config.Addr。
func (a *Adapter) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.host == nil {
		return fmt.Errorf("http adapter not mounted")
	}
	if strings.TrimSpace(a.addr) == "" {
		return fmt.Errorf("http.Module requires http.Config{Addr: \":8080\"}")
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

// WithBasePath 为 adapter 设置统一的路由前缀。
func (a *Adapter) WithBasePath(prefix string) *Adapter {
	a.basePath = prefix
	return a
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

// HandleHTTP 是 MountHandler 的无返回值形式，供 adapter.Mounter 使用。
func (a *Adapter) HandleHTTP(pattern string, handler http.Handler) {
	a.MountHandler(pattern, handler)
}

// Static 是 ServeStatic 的无返回值形式，供 adapter.Mounter 使用。
func (a *Adapter) Static(prefix, root string) {
	a.ServeStatic(prefix, root)
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
