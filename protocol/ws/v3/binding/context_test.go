package ws

import (
	"net"
	"net/http"
	"net/url"
	"testing"

	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

type fakeAddr struct{ v string }

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return a.v }

type fakeConn struct {
	ctx   any
	url   url.URL
	rooms []string
}

func (c *fakeConn) ID() string                  { return "id" }
func (c *fakeConn) URL() url.URL                { return c.url }
func (c *fakeConn) RemoteAddr() net.Addr        { return fakeAddr{v: "1.2.3.4:5"} }
func (c *fakeConn) Context() any                { return c.ctx }
func (c *fakeConn) Emit(event string, v ...any) {}
func (c *fakeConn) Join(room string)            { c.rooms = append(c.rooms, room) }
func (c *fakeConn) Leave(room string)           {}
func (c *fakeConn) LeaveAll()                   { c.rooms = nil }
func (c *fakeConn) Rooms() []string             { return append([]string{}, c.rooms...) }
func (c *fakeConn) Close() error                { return nil }
func (c *fakeConn) Request() *http.Request {
	if r, ok := c.ctx.(*http.Request); ok {
		return r
	}
	return nil
}

type fakeServer struct{}

func (s fakeServer) BroadcastToRoom(namespace string, room string, event string, args ...any) bool {
	return true
}

func (s fakeServer) BroadcastToNamespace(namespace string, event string, args ...any) bool {
	return true
}

func (s fakeServer) RoomLen(namespace string, room string) int { return 7 }

type chatMessageDTO struct {
	Room string `json:"room" validate:"required"`
	Text string `json:"text" validate:"required,min=2"`
}

func TestWsContext_Basics(t *testing.T) {
	ctx := runtime.NewHandlerContext("ws")
	ctx.Request.Method = "evt"
	ctx.Request.Path = "/n"
	ctx.Set(MetadataKeyEvent, "evt")
	ctx.Set(MetadataKeyServer, fakeServer{})

	req := &http.Request{URL: &url.URL{Scheme: "http", Host: "x", Path: "/socket.io/", RawQuery: "a=1"}, RemoteAddr: "9.9.9.9:9", Header: http.Header{"Authorization": []string{"t"}}}
	c := &fakeConn{ctx: req, url: *req.URL, rooms: []string{"r"}}
	ctx.Set(MetadataKeySocket, c)
	ctx.Set(MetadataKeyHeaders, req.Header)

	w := NewWsContext(ctx)
	if w.Event() != "evt" || w.Namespace() != "/n" {
		t.Fatalf("event/ns")
	}
	if w.ID() != "id" {
		t.Fatalf("id")
	}
	if w.Query("a") != "1" {
		t.Fatalf("query")
	}
	if w.RemoteAddr() == "" {
		t.Fatalf("remote")
	}
	if w.RoomLen("x") != 7 {
		t.Fatalf("roomlen")
	}
	if err := w.BroadcastTo("r", "e"); err != nil {
		t.Fatalf("broadcast: %v", err)
	}
	if err := w.BroadcastToNamespace("e"); err != nil {
		t.Fatalf("broadcast ns: %v", err)
	}
}

func TestWsContext_ShouldBindMessage(t *testing.T) {
	ctx := runtime.NewHandlerContext("ws")
	ctx.Request.BodyBytes = []byte(`{"room":"general","text":"hello"}`)

	w := NewWsContext(ctx)
	var dto chatMessageDTO
	if err := w.ShouldBindMessage(&dto); err != nil {
		t.Fatalf("ShouldBindMessage() error = %v", err)
	}
	if dto.Room != "general" || dto.Text != "hello" {
		t.Fatalf("unexpected dto: %#v", dto)
	}
}

func TestWsContext_ShouldBindMessageValidationError(t *testing.T) {
	ctx := runtime.NewHandlerContext("ws")
	ctx.Request.BodyBytes = []byte(`{"room":"","text":"x"}`)

	w := NewWsContext(ctx)
	var dto chatMessageDTO
	err := w.ShouldBindMessage(&dto)
	if err == nil {
		t.Fatalf("ShouldBindMessage() expected validation error")
	}
	if ke, ok := err.(*kerrors.KernelError); !ok || ke.Kind != kerrors.KindValidation {
		t.Fatalf("ShouldBindMessage() error kind = %#v, want validation kernel error", err)
	}
}
