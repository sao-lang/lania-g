// socket_conn.go 把 zishang520/socket.io 的 *socket.Socket 适配成 binding/ws 依赖的最小接口面。
//
// 这样做的好处：
// - binding/ws 不需要感知具体 socket.io 实现
// - 替换底层库时只改这一层
package ws

import (
	"net"
	"net/http"
	"net/url"
	"sync"

	"github.com/zishang520/socket.io/socket"
)

// socketConn 是 socket.io 连接的一层轻量适配器。
//
// 它同时满足 binding/ws 中通过类型断言使用的接口：
// ID / URL / RemoteAddr / Request / Emit / Join / Leave / Rooms / LeaveAll / Close。
type socketConn struct {
	socket *socket.Socket
	header http.Header
	req    *http.Request
	addr   net.Addr

	mu      sync.RWMutex
	context any
}

var (
	_ interface{ ID() string }                 = (*socketConn)(nil)
	_ interface{ URL() url.URL }               = (*socketConn)(nil)
	_ interface{ RemoteAddr() net.Addr }       = (*socketConn)(nil)
	_ interface{ Request() *http.Request }     = (*socketConn)(nil)
	_ interface{ Emit(string, ...any) }        = (*socketConn)(nil)
	_ interface{ Join(string) }                = (*socketConn)(nil)
	_ interface{ Leave(string) }               = (*socketConn)(nil)
	_ interface{ Rooms() []string }            = (*socketConn)(nil)
	_ interface{ LeaveAll() }                  = (*socketConn)(nil)
	_ interface{ Close() error }               = (*socketConn)(nil)
)

func newSocketConn(s *socket.Socket) *socketConn {
	conn := &socketConn{
		socket: s,
		header: http.Header{},
		addr:   wsAddr(""),
	}

	hs := s.Handshake()
	if hs == nil {
		conn.req = &http.Request{Method: http.MethodGet, URL: &url.URL{}}
		return conn
	}

	if hs.Headers != nil {
		for k, v := range hs.Headers.All() {
			conn.header[k] = append([]string{}, v...)
		}
	}

	u := &url.URL{}
	if hs.Url != "" {
		if parsed, err := url.Parse(hs.Url); err == nil {
			u = parsed
		}
	}
	if u.RawQuery == "" && hs.Query != nil {
		u.RawQuery = url.Values(hs.Query.All()).Encode()
	}

	conn.addr = wsAddr(hs.Address)
	host := conn.header.Get("Host")
	if host == "" {
		host = hs.Address
	}
	conn.req = &http.Request{
		Method:     http.MethodGet,
		URL:        u,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     conn.header,
		Host:       host,
		RemoteAddr: hs.Address,
	}
	return conn
}

// ID 返回连接 id。
func (c *socketConn) ID() string {
	if c == nil || c.socket == nil {
		return ""
	}
	return string(c.socket.Id())
}

// URL 返回握手请求 URL。
func (c *socketConn) URL() url.URL {
	if c == nil || c.req == nil || c.req.URL == nil {
		return url.URL{}
	}
	return *c.req.URL
}

// RemoteAddr 返回客户端地址。
func (c *socketConn) RemoteAddr() net.Addr {
	if c == nil {
		return wsAddr("")
	}
	return c.addr
}

// Request 返回由握手中继出来的 HTTP 请求视图。
func (c *socketConn) Request() *http.Request {
	if c == nil {
		return nil
	}
	return c.req
}

// RemoteHeader 返回握手请求头。
func (c *socketConn) RemoteHeader() http.Header {
	if c == nil {
		return nil
	}
	return c.header
}

// SetContext / Context 兼容旧的连接上下文语义。
func (c *socketConn) SetContext(v any) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.context = v
	c.mu.Unlock()
}

// Context 返回连接上下文。
func (c *socketConn) Context() any {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.context
}

// Emit 向当前连接发送事件。
//
// 注意：为了满足 binding/ws 的最小接口（`interface{ Emit(string, ...any) }`），
// 这里不返回 error；需要感知发送失败时请直接使用底层 socket。
func (c *socketConn) Emit(event string, args ...any) {
	if c == nil || c.socket == nil {
		return
	}
	_ = c.socket.Emit(event, args...)
}

// Join 加入房间。
func (c *socketConn) Join(room string) {
	if c == nil || c.socket == nil {
		return
	}
	c.socket.Join(socket.Room(room))
}

// Leave 离开房间。
func (c *socketConn) Leave(room string) {
	if c == nil || c.socket == nil {
		return
	}
	c.socket.Leave(socket.Room(room))
}

// Rooms 返回当前连接所在房间。
func (c *socketConn) Rooms() []string {
	if c == nil || c.socket == nil {
		return nil
	}
	keys := c.socket.Rooms().Keys()
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, string(k))
	}
	return out
}

// LeaveAll 离开所有房间。
func (c *socketConn) LeaveAll() {
	if c == nil || c.socket == nil {
		return
	}
	for _, room := range c.socket.Rooms().Keys() {
		c.socket.Leave(room)
	}
}

// Close 主动断开连接。
func (c *socketConn) Close() error {
	if c == nil || c.socket == nil {
		return nil
	}
	c.socket.Disconnect(true)
	return nil
}

// wsAddr 是 net.Addr 的最小实现（仅用于暴露远端地址字符串）。
type wsAddr string

func (a wsAddr) Network() string { return "tcp" }
func (a wsAddr) String() string  { return string(a) }

// ---- Adapter 侧的广播能力（由 binding/ws 通过 MetadataKeyServer 使用）----

// BroadcastToRoom 向指定 namespace 下某个房间广播事件。
func (a *Adapter) BroadcastToRoom(namespace, room, event string, args ...any) bool {
	if a == nil || a.server == nil {
		return false
	}
	nsp := a.server.Of(namespace, nil)
	if nsp == nil {
		return false
	}
	return nsp.To(socket.Room(room)).Emit(event, args...) == nil
}

// BroadcastToNamespace 向整个 namespace 广播事件。
func (a *Adapter) BroadcastToNamespace(namespace, event string, args ...any) bool {
	if a == nil || a.server == nil {
		return false
	}
	nsp := a.server.Of(namespace, nil)
	if nsp == nil {
		return false
	}
	return nsp.Emit(event, args...) == nil
}

// RoomLen 返回指定房间内的连接数。
func (a *Adapter) RoomLen(namespace, room string) int {
	if a == nil || a.server == nil {
		return 0
	}
	nsp := a.server.Of(namespace, nil)
	if nsp == nil {
		return 0
	}
	return len(nsp.To(socket.Room(room)).FetchSockets())
}
