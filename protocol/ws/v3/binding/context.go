// context.go 定义 WS binding 使用的上下文适配层。
package ws

import (
	"errors"
	"net"
	"net/http"
	"net/url"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// Context 定义 WebSocket handler 可注入的上下文能力。
// 它的目标不是抽象所有 socket.io API，而是暴露框架里最常用、最稳定的一小组能力。
type Context interface {
	HandlerContext() *runtime.HandlerContext
	Event() string
	Namespace() string
	ID() string
	URL() string
	RemoteAddr() string
	Query(key string) string
	Request() *http.Request
	Headers() http.Header
	Message() any
	Conn() any
	Server() any

	Emit(event string, args ...any) error
	Join(room string) error
	Leave(room string) error
	Rooms() []string
	BroadcastTo(room, event string, args ...any) error
	BroadcastToNamespace(event string, args ...any) error
	RoomLen(room string) int
	LeaveAll() error
	Disconnect() error
	ShouldBindMessage(obj any) error
}

// WsContext 是 `binding/ws.Context` 的默认实现。
// 它本质上是对 runtime.HandlerContext + socket.io 连接对象的一层运行时投影。
type WsContext struct {
	ctx *runtime.HandlerContext
}

// NewWsContext 基于 runtime.HandlerContext 创建一个 WebSocket 上下文包装。
func NewWsContext(ctx *runtime.HandlerContext) *WsContext {
	return &WsContext{ctx: ctx}
}

// HandlerContext 返回底层 runtime.HandlerContext。
func (w *WsContext) HandlerContext() *runtime.HandlerContext { return w.ctx }

// Event 返回当前消息对应的事件名。
// 优先读 adapter 预写入的 metadata；没有时回退到 runtime.Request.Method。
func (w *WsContext) Event() string {
	if w == nil || w.ctx == nil {
		return ""
	}
	if v, ok := w.ctx.Get(MetadataKeyEvent); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return w.ctx.Request.Method
}

// Namespace 返回当前连接所属的命名空间。
// 这里直接复用 runtime.Request.Path 作为 namespace 载体。
func (w *WsContext) Namespace() string {
	if w == nil || w.ctx == nil {
		return ""
	}
	return w.ctx.Request.Path
}

// ID 返回当前连接的连接标识；若底层连接不支持则返回空串。
func (w *WsContext) ID() string {
	conn := w.Conn()
	if conn == nil {
		return ""
	}
	if ider, ok := conn.(interface{ ID() string }); ok {
		return ider.ID()
	}
	return ""
}

// URL 返回当前连接关联的 URL。
func (w *WsContext) URL() string {
	conn := w.Conn()
	if conn == nil {
		return ""
	}
	if urler, ok := conn.(interface{ URL() url.URL }); ok {
		u := urler.URL()
		return u.String()
	}
	if req := w.Request(); req != nil {
		return req.URL.String()
	}
	return ""
}

// RemoteAddr 返回客户端远端地址（尽量从底层连接读取，否则回退到 HTTP 请求）。
func (w *WsContext) RemoteAddr() string {
	conn := w.Conn()
	if conn == nil {
		return ""
	}
	if ra, ok := conn.(interface{ RemoteAddr() net.Addr }); ok {
		if addr := ra.RemoteAddr(); addr != nil {
			return addr.String()
		}
	}
	if req := w.Request(); req != nil {
		return req.RemoteAddr
	}
	return ""
}

// Query 从 URL 查询参数中读取 key 对应的值。
func (w *WsContext) Query(key string) string {
	if key == "" {
		return ""
	}
	conn := w.Conn()
	if conn != nil {
		if urler, ok := conn.(interface{ URL() url.URL }); ok {
			u := urler.URL()
			return u.Query().Get(key)
		}
	}
	if req := w.Request(); req != nil {
		return req.URL.Query().Get(key)
	}
	return ""
}

// Request 返回底层连接关联的 HTTP 握手请求（若底层连接不支持则返回 nil）。
// 这里兼容两种常见风格：连接对象直接暴露 `Request()`，或把请求塞进 `Context()`。
func (w *WsContext) Request() *http.Request {
	conn := w.Conn()
	if conn == nil {
		return nil
	}
	if reqer, ok := conn.(interface{ Request() *http.Request }); ok {
		return reqer.Request()
	}
	if c, ok := conn.(interface{ Context() any }); ok {
		if req, ok := c.Context().(*http.Request); ok {
			return req
		}
	}
	return nil
}

// Headers 返回请求头；优先使用上下文中注入的 headers 元信息。
// 返回的是副本视图，避免业务层直接修改 runtime 内部 header 存储。
func (w *WsContext) Headers() http.Header {
	if w == nil || w.ctx == nil {
		return nil
	}
	if v, ok := w.ctx.Get(MetadataKeyHeaders); ok {
		if h, ok := v.(http.Header); ok {
			return h
		}
	}
	out := http.Header{}
	for k, v := range w.ctx.Request.HeadersMulti {
		out[k] = append([]string{}, v...)
	}
	return out
}

// Message 返回当前消息体（可能是任意类型，通常为 `[]byte`/`string`/结构体等）。
// 优先保留 adapter 原样放进来的 `Request.Body`，避免过早丢失消息原始类型信息。
func (w *WsContext) Message() any {
	if w == nil || w.ctx == nil {
		return nil
	}
	if w.ctx.Request.Body != nil {
		return w.ctx.Request.Body
	}
	if len(w.ctx.Request.BodyBytes) > 0 {
		return w.ctx.Request.BodyBytes
	}
	return nil
}

// Conn 返回底层连接对象（由 WS adapter 写入到上下文元信息）。
func (w *WsContext) Conn() any {
	if w == nil || w.ctx == nil {
		return nil
	}
	if v, ok := w.ctx.Get(MetadataKeySocket); ok {
		return v
	}
	return nil
}

// Server 返回底层 WS server 对象（由 WS adapter 写入到上下文元信息）。
func (w *WsContext) Server() any {
	if w == nil || w.ctx == nil {
		return nil
	}
	if v, ok := w.ctx.Get(MetadataKeyServer); ok {
		return v
	}
	return nil
}

// ShouldBindMessage 把当前事件消息绑定到 obj，并在可用时执行 DTO 校验。
func (w *WsContext) ShouldBindMessage(obj any) error {
	if w == nil || w.ctx == nil {
		return errors.New("ws context is nil")
	}
	if err := BindInto(w.ctx, obj); err != nil {
		return err
	}
	v := validatorFromContext(w.ctx)
	if v == nil {
		v = defaultBindValidator()
	}
	if v == nil {
		return nil
	}
	return v.Validate(obj)
}

// Emit 向当前连接发送一个事件。
// 这里刻意只依赖极小的接口面，避免 WsContext 和具体 socket.io 连接类型强耦合。
func (w *WsContext) Emit(event string, args ...any) error {
	conn := w.Conn()
	if conn == nil {
		return errors.New("ws conn is nil")
	}
	if e, ok := conn.(interface{ Emit(string, ...any) }); ok {
		e.Emit(event, args...)
		return nil
	}
	return errors.New("ws conn does not support Emit")
}

// Join 让当前连接加入指定房间。
func (w *WsContext) Join(room string) error {
	conn := w.Conn()
	if conn == nil {
		return errors.New("ws conn is nil")
	}
	if j, ok := conn.(interface{ Join(string) }); ok {
		j.Join(room)
		return nil
	}
	return errors.New("ws conn does not support Join")
}

// Leave 让当前连接离开指定房间。
func (w *WsContext) Leave(room string) error {
	conn := w.Conn()
	if conn == nil {
		return errors.New("ws conn is nil")
	}
	if l, ok := conn.(interface{ Leave(string) }); ok {
		l.Leave(room)
		return nil
	}
	return errors.New("ws conn does not support Leave")
}

// Rooms 返回当前连接所在的房间列表。
func (w *WsContext) Rooms() []string {
	conn := w.Conn()
	if conn == nil {
		return nil
	}
	if r, ok := conn.(interface{ Rooms() []string }); ok {
		return append([]string{}, r.Rooms()...)
	}
	return nil
}

// BroadcastTo 向指定房间内的连接广播一个事件。
// 广播时总是带上当前 namespace，避免不同 namespace 的 room 名冲突。
func (w *WsContext) BroadcastTo(room, event string, args ...any) error {
	server := w.Server()
	if server == nil {
		return errors.New("ws server is nil")
	}
	ns := w.Namespace()
	if b, ok := server.(interface {
		BroadcastToRoom(string, string, string, ...any) bool
	}); ok {
		if b.BroadcastToRoom(ns, room, event, args...) {
			return nil
		}
		return errors.New("ws broadcast failed")
	}
	return errors.New("ws server does not support BroadcastToRoom")
}

// BroadcastToNamespace 向当前命名空间内的所有连接广播一个事件。
func (w *WsContext) BroadcastToNamespace(event string, args ...any) error {
	server := w.Server()
	if server == nil {
		return errors.New("ws server is nil")
	}
	ns := w.Namespace()
	if b, ok := server.(interface {
		BroadcastToNamespace(string, string, ...any) bool
	}); ok {
		if b.BroadcastToNamespace(ns, event, args...) {
			return nil
		}
		return errors.New("ws broadcast failed")
	}
	return errors.New("ws server does not support BroadcastToNamespace")
}

// RoomLen 返回指定房间内的连接数（若底层 server 不支持则返回 0）。
func (w *WsContext) RoomLen(room string) int {
	server := w.Server()
	if server == nil {
		return 0
	}
	ns := w.Namespace()
	if rl, ok := server.(interface{ RoomLen(string, string) int }); ok {
		return rl.RoomLen(ns, room)
	}
	return 0
}

// LeaveAll 让当前连接离开所有房间。
func (w *WsContext) LeaveAll() error {
	conn := w.Conn()
	if conn == nil {
		return errors.New("ws conn is nil")
	}
	if la, ok := conn.(interface{ LeaveAll() }); ok {
		la.LeaveAll()
		return nil
	}
	return errors.New("ws conn does not support LeaveAll")
}

// Disconnect 主动断开当前连接。
func (w *WsContext) Disconnect() error {
	conn := w.Conn()
	if conn == nil {
		return errors.New("ws conn is nil")
	}
	if c, ok := conn.(interface{ Close() error }); ok {
		return c.Close()
	}
	return errors.New("ws conn does not support Disconnect")
}
