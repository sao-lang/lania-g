// ws.go 实现 WS adapter 的主入口与宿主集成逻辑。
package ws

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	wsbinding "github.com/sao-lang/lania-g/protocol/ws/v3/binding"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	wsprotocol "github.com/sao-lang/lania-g/protocol/ws/v3/protocol"

	socketio "github.com/googollee/go-socket.io"
	"github.com/googollee/go-socket.io/engineio"
)

// AdapterID 是 WebSocket 协议插件与 adapter 的统一标识。
const AdapterID = "ws"

// Adapter 是 WebSocket(socket.io) 协议的运行期适配器，负责挂载 server 并把事件分发到 runtime.Execute。
type Adapter struct {
	api  *API
	host adapter.Host

	server *socketio.Server
	path   string
	addr   string

	gatewayMu    sync.RWMutex
	gatewaysByNS map[string][]gatewayRef

	nsMu                 sync.Mutex
	registeredNamespaces map[string]bool
	registeredEvents     map[string]map[string]string

	serveMu    sync.Mutex
	serving    bool
	standalone bool

	responseSuffix string
	errorSuffix    string

	mountedToHTTP bool
	listener      net.Listener
	httpServer    *http.Server

	mu sync.Mutex
}

var _ adapter.Adapter = (*Adapter)(nil)

// New 创建 WS adapter。
// - `New()` 使用共享监听模式：挂到共享 HTTP listener 上，需要 HTTP adapter + `app.Listen(...)`
// - `New(":3000")` 使用独立监听模式：adapter 自己启动一个 HTTP 服务
//
// 这是该 adapter 唯一的公开端口绑定入口。
func New(addr ...string) *Adapter {
	s := socketio.NewServer(&engineio.Options{
		PingInterval: 25 * time.Second,
		PingTimeout:  60 * time.Second,
	})
	a := &Adapter{
		server:               s,
		path:                 "/socket.io/",
		api:                  NewCompatAPI(),
		gatewaysByNS:         make(map[string][]gatewayRef),
		registeredNamespaces: make(map[string]bool),
		registeredEvents:     make(map[string]map[string]string),
		responseSuffix:       responseEventSuffix,
		errorSuffix:          errorEventSuffix,
	}
	if len(addr) > 0 && strings.TrimSpace(addr[0]) != "" {
		a.standalone = true
		a.addr = addr[0]
	}
	return a
}

// Listen 仅为兼容旧写法保留。
// Deprecated: 请改用 `ws.New(":addr")`。
func Listen(addr string) *Adapter { return New(addr) }

// ID 返回该 adapter 的唯一标识（用于 registry/plugin 分组）。
func (a *Adapter) ID() string { return AdapterID }

// API 返回该 adapter 暴露给应用侧的 DSL API（用于声明 WS gateway/handlers）。
func (a *Adapter) API() any { return a.api }

// Plugins 返回该 adapter 参与编译的协议插件列表。
func (a *Adapter) Plugins() []compiler.ProtocolPlugin {
	return []compiler.ProtocolPlugin{NewPlugin()}
}

// Mount 将 adapter 挂载到应用 host。
//
// Mount 阶段会：
// - 保存 host 引用
// - 使用 host.Registry() 初始化 API，使 DSL 写入与当前应用的 registry 绑定
// - 尝试挂载到共享 HTTP listener（如果 host 支持 HTTPMountHost）
func (a *Adapter) Mount(host adapter.Host) error {
	a.host = host
	a.api = NewAPI(host.Registry())
	a.mountedToHTTP = a.tryMountToSharedHTTP(host) == nil

	return nil
}

// Start 启动 adapter。
//
// 两种模式：
// - shared listen：挂载到 HTTP adapter 的共享 listener（需要先挂载 HTTP adapter 并调用 app.Listen）
// - standalone：ws.New(":port")，adapter 自己起一个 http.Server 来承载 socket.io
//
// Start 内部会调用 Reload() 注册 routes 与 hooks。
func (a *Adapter) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.host == nil {
		return fmt.Errorf("ws adapter not mounted")
	}
	if a.server == nil {
		return fmt.Errorf("ws server is nil")
	}

	if err := a.tryMountToSharedHTTP(a.host); err == nil {
		a.mountedToHTTP = true
	}

	if !a.mountedToHTTP && !a.standalone {
		return fmt.Errorf("ws adapter not mounted to http: mount http adapter or use ws.New(\":addr\")")
	}

	if err := a.Reload(); err != nil {
		return err
	}

	if a.standalone {
		if strings.TrimSpace(a.addr) == "" {
			return fmt.Errorf("ws standalone adapter requires addr")
		}
		if a.httpServer != nil || a.listener != nil {
			return nil
		}
		a.ensureServing()
		return a.startStandaloneLocked()
	}
	a.ensureServing()
	return nil
}

// WithAddr 仅为兼容旧写法保留。
// Deprecated: 请改用 `ws.New(":addr")`。
func (a *Adapter) WithAddr(addr string) *Adapter {
	a.standalone = true
	a.addr = addr
	return a
}

// AsStandalone 仅为兼容旧写法保留。
// Deprecated: 请改用 `ws.New(":addr")`。
func (a *Adapter) AsStandalone(addr string) *Adapter {
	a.standalone = true
	a.addr = addr
	return a
}

// tryMountToSharedHTTP 尝试将 socket.io server 挂载到共享 HTTP adapter。
//
// 需要 host 实现 adapter.HTTPMountHost；否则返回错误（由上层决定是否 fallback 到 standalone）。
func (a *Adapter) tryMountToSharedHTTP(host adapter.Host) error {
	if host == nil {
		return fmt.Errorf("host is nil")
	}
	if a.server == nil {
		return fmt.Errorf("ws server is nil")
	}
	mh, ok := host.(adapter.HTTPMountHost)
	if !ok {
		return fmt.Errorf("host does not support http mount")
	}
	return mh.MountHTTP(a.path, a.server)
}

// WithResponseEventSuffix 设置“响应事件”的后缀（默认 Response）。
//
// 例如 event="Ping"，则响应事件为 "Ping" + suffix。
func (a *Adapter) WithResponseEventSuffix(suffix string) *Adapter {
	a.mu.Lock()
	defer a.mu.Unlock()
	if suffix == "" {
		return a
	}
	a.responseSuffix = suffix
	return a
}

// WithErrorEventSuffix 设置“错误事件”的后缀（默认 Error）。
//
// 当 handler 返回 error 时，会 emit event+suffix，并携带错误消息。
func (a *Adapter) WithErrorEventSuffix(suffix string) *Adapter {
	a.mu.Lock()
	defer a.mu.Unlock()
	if suffix == "" {
		return a
	}
	a.errorSuffix = suffix
	return a
}

// ensureServing 确保 socket.io server 的 Serve() 只启动一次（后台 goroutine）。
func (a *Adapter) ensureServing() {
	a.serveMu.Lock()
	defer a.serveMu.Unlock()
	if a.serving {
		return
	}
	a.serving = true
	go func() {
		_ = a.server.Serve()
	}()
}

// startStandaloneLocked 在 standalone 模式下启动一个独立的 http.Server。
//
// 注意：调用方需持有 a.mu（Start 中已持有），避免并发启动多个 listener。
func (a *Adapter) startStandaloneLocked() error {
	ln, err := net.Listen("tcp", a.addr)
	if err != nil {
		return err
	}
	a.listener = ln
	mux := http.NewServeMux()
	mux.Handle(a.path, a.server)
	a.httpServer = &http.Server{Addr: a.addr, Handler: mux}
	go func() {
		_ = a.httpServer.Serve(ln)
	}()
	return nil
}

// Reload 重新加载运行时路由并注册 socket.io 事件回调。
//
// 目前实现会：
// - collectGatewaysByNamespace：从编译期声明推导 namespace -> gateways（用于连接/断开等 hooks）
// - registerRoutesFromRuntimeDelta：从 runtime.Router 拉取 WS routes 并注册 OnEvent
func (a *Adapter) Reload() error {
	a.collectGatewaysByNamespace()
	return a.registerRoutesFromRuntimeDelta()
}

// Stop 停止 adapter，并清理内部状态（listener/server/注册表/缓存）。
func (a *Adapter) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.httpServer != nil {
		_ = a.httpServer.Close()
		a.httpServer = nil
	}
	if a.listener != nil {
		_ = a.listener.Close()
		a.listener = nil
	}
	if a.server != nil {
		a.server.Close()
	}
	a.serveMu.Lock()
	a.serving = false
	a.serveMu.Unlock()

	a.gatewayMu.Lock()
	for k := range a.gatewaysByNS {
		delete(a.gatewaysByNS, k)
	}
	a.gatewayMu.Unlock()

	a.nsMu.Lock()
	for k := range a.registeredNamespaces {
		delete(a.registeredNamespaces, k)
	}
	for k := range a.registeredEvents {
		delete(a.registeredEvents, k)
	}
	a.nsMu.Unlock()

	a.mountedToHTTP = false
	a.standalone = false
	return nil
}

// Handler 返回 socket.io server 作为 http.Handler，供 HTTP adapter mount。
func (a *Adapter) Handler() http.Handler {
	return a.server
}

// WithSocketPath 设置 socket.io 的挂载路径（默认 `/socket.io/`）。
//
// 会自动补齐前导 `/` 与尾部 `/`，以满足 socket.io 的路径约定。
func (a *Adapter) WithSocketPath(path string) *Adapter {
	a.mu.Lock()
	defer a.mu.Unlock()
	if path == "" {
		return a
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	a.path = path
	return a
}

const (
	responseEventSuffix = "Response"
	errorEventSuffix    = "Error"
)

// registerRoutesFromRuntimeDelta 从 runtime.Router 读取已经编译好的 WS routes，
// 并把它们翻译成 socket.io 的 `OnEvent(namespace, event, handler)` 注册。
func (a *Adapter) registerRoutesFromRuntimeDelta() error {
	rt := a.host.Runtime()
	if rt == nil {
		return nil
	}

	routes := rt.GetRouter().AllRoutes()
	for key := range routes {
		rk, err := runtime.ParseRouteKey(key)
		if err != nil {
			continue
		}
		if rk.Protocol != wsprotocol.Protocol {
			continue
		}

		ns := normalizeNamespace(rk.Path)
		event := rk.Method
		routeKey := key

		if err := a.ensureNamespaceHooks(ns); err != nil {
			return err
		}
		ok, err := a.markEventRegistered(ns, event, routeKey)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}

		a.server.OnEvent(ns, event, func(conn socketio.Conn, msg any) {
			ctx := runtime.AcquireHandlerContext(wsprotocol.Protocol)
			defer runtime.ReleaseHandlerContext(ctx)

			// 把 socket.io 回调现场投影成框架统一的 HandlerContext，
			// 后续就能直接复用 runtime.Execute + binding/ws 整套参数解析。
			ctx.Request.Method = event
			ctx.Request.Path = ns
			ctx.RouteKey = routeKey
			ctx.Set(wsbinding.MetadataKeySocket, conn)
			ctx.Set(wsbinding.MetadataKeyServer, a.server)
			ctx.Set(wsbinding.MetadataKeyEvent, event)

			h := conn.RemoteHeader()
			if h != nil {
				ctx.Set(wsbinding.MetadataKeyHeaders, h)
				for k, v := range h {
					ctx.Request.HeadersMulti[k] = append([]string{}, v...)
					if len(v) > 0 {
						ctx.Request.Headers[k] = v[0]
					}
				}
			}

			ctx.Request.Body = msg
			if b, err := jsonBytes(msg); err == nil {
				// BodyBytes 主要服务 `WSMessageBody[T]` / BindInto 这类按 JSON 处理的路径。
				ctx.Request.BodyBytes = b
			}

			result, execErr := rt.Execute(ctx)
			if execErr != nil {
				errSuffix := a.errorSuffix
				if errSuffix == "" {
					errSuffix = errorEventSuffix
				}
				conn.Emit(event+errSuffix, execErr.Error())
				return
			}
			if result != nil {
				respSuffix := a.responseSuffix
				if respSuffix == "" {
					respSuffix = responseEventSuffix
				}
				conn.Emit(event+respSuffix, result)
			}
		})
	}
	return nil
}

// ensureNamespaceHooks 确保某个 namespace 的 connect/disconnect/error hooks 只注册一次。
//
// 注意：
// - registeredNamespaces/registeredEvents 用于去重与冲突检测
// - hooks 的回调会调用对应 gateway 的 OnWebSocket* 接口方法
func (a *Adapter) ensureNamespaceHooks(ns string) error {
	a.nsMu.Lock()
	if a.registeredNamespaces[ns] {
		a.nsMu.Unlock()
		return nil
	}
	a.registeredNamespaces[ns] = true
	if a.registeredEvents[ns] == nil {
		a.registeredEvents[ns] = make(map[string]string)
	}
	a.nsMu.Unlock()

	a.server.OnConnect(ns, func(conn socketio.Conn) error {
		conn.SetContext(nil)
		return a.callOnConnect(ns, conn)
	})
	a.server.OnDisconnect(ns, func(conn socketio.Conn, reason string) {
		a.callOnDisconnect(ns, conn, reason)
	})
	a.server.OnError(ns, func(conn socketio.Conn, err error) {
		a.callOnError(ns, conn, err)
	})
	return nil
}

// markEventRegistered 记录某个 namespace 下 event -> routeKey 的注册关系。
//
// 用途：
// - 防止重复注册同一个事件回调
// - 检测冲突：同一 namespace+event 对应不同 routeKey 时返回错误
func (a *Adapter) markEventRegistered(ns, event, routeKey string) (bool, error) {
	a.nsMu.Lock()
	defer a.nsMu.Unlock()
	if a.registeredEvents[ns] == nil {
		a.registeredEvents[ns] = make(map[string]string)
	}
	if existing, ok := a.registeredEvents[ns][event]; ok {
		if existing != routeKey {
			return false, fmt.Errorf("duplicate ws route for namespace=%s event=%s", ns, event)
		}
		return false, nil
	}
	a.registeredEvents[ns][event] = routeKey
	return true, nil
}

// collectGatewaysByNamespace 从 registry 声明中构建 gatewaysByNS 索引。
//
// 该索引用于 connect/disconnect/error hooks：当某个 namespace 有连接事件时，
// 会遍历该 namespace 下所有 gateways，调用对应的 OnWebSocket* hook。
func (a *Adapter) collectGatewaysByNamespace() {
	if a.host == nil {
		return
	}
	reg := a.host.Registry()
	if reg == nil {
		return
	}
	moduleRef := a.host.ModuleRef()
	if moduleRef == nil {
		return
	}
	items := reg.ListDecl(AdapterID, "handlers")
	refs, err := buildGatewayRefs(moduleRef, items)
	if err != nil {
		return
	}
	a.gatewayMu.Lock()
	a.gatewaysByNS = refs
	a.gatewayMu.Unlock()
}

// callOnConnect 触发某个 namespace 下所有 gateway 的 OnWebSocketConnect hook。
// 这里按 namespace 找到所有 gateway，而不是只找某个 event 对应的 handler。
func (a *Adapter) callOnConnect(ns string, conn any) error {
	a.gatewayMu.RLock()
	gateways := append([]gatewayRef{}, a.gatewaysByNS[ns]...)
	a.gatewayMu.RUnlock()
	for _, ref := range gateways {
		gw := ref.resolve()
		if gw == nil {
			continue
		}
		if hook, ok := gw.(OnWebSocketConnect); ok {
			if err := hook.OnWebSocketConnect(conn); err != nil {
				return err
			}
		}
	}
	return nil
}

// callOnDisconnect 触发某个 namespace 下所有 gateway 的 OnWebSocketDisconnect hook。
func (a *Adapter) callOnDisconnect(ns string, conn any, reason string) {
	a.gatewayMu.RLock()
	gateways := append([]gatewayRef{}, a.gatewaysByNS[ns]...)
	a.gatewayMu.RUnlock()
	for _, ref := range gateways {
		gw := ref.resolve()
		if gw == nil {
			continue
		}
		if hook, ok := gw.(OnWebSocketDisconnect); ok {
			hook.OnWebSocketDisconnect(conn, reason)
		}
	}
}

// callOnError 触发某个 namespace 下所有 gateway 的 OnWebSocketError hook。
func (a *Adapter) callOnError(ns string, conn any, err error) {
	a.gatewayMu.RLock()
	gateways := append([]gatewayRef{}, a.gatewaysByNS[ns]...)
	a.gatewayMu.RUnlock()
	for _, ref := range gateways {
		gw := ref.resolve()
		if gw == nil {
			continue
		}
		if hook, ok := gw.(OnWebSocketError); ok {
			hook.OnWebSocketError(conn, err)
		}
	}
}

// normalizeNamespace 规范化 WS namespace 前缀：
// - 空串 -> "/"
// - 自动补齐前导 "/"
// - 非根路径去掉尾部 "/"
func normalizeNamespace(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "/"
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	if len(prefix) > 1 && strings.HasSuffix(prefix, "/") {
		prefix = strings.TrimSuffix(prefix, "/")
	}
	return prefix
}

// jsonBytes 将事件消息编码为 JSON bytes（用于填充 ctx.Request.BodyBytes）。
//
// 行为：
// - nil -> (nil, nil)
// - []byte/string -> 直接返回（拷贝/转换）
// - 其他 -> json.Marshal
func jsonBytes(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	switch x := v.(type) {
	case []byte:
		return append([]byte{}, x...), nil
	case string:
		return []byte(x), nil
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return nil, err
		}
		return b, nil
	}
}
