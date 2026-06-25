// context.go 定义 runtime 层统一的请求上下文与协议抽象。
//
// 这一层的目标是把 HTTP/WS/gRPC/MQ/Scheduler 等协议先投影成同一套结构，
// 后续的 binding、AOP、executor 才能复用同一套执行逻辑。
package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sao-lang/lania-g/kernel/v3/di"
)

// Protocol 表示 runtime 视角下的协议标识，例如 `http`、`grpc`、`ws`。
type Protocol string

// HandlerContext 是 runtime 的“统一请求上下文”，用于屏蔽 HTTP/WS/gRPC/GraphQL 的差异。
//
// 设计要点：
// - Request/Response 是框架内部的统一抽象；适配器负责把协议原生对象映射到这里
// - Container 是 request-scope 的 DI 容器（通常由 Executor 在 Execute 时创建 child）
// - Metadata 用于跨组件透传（例如 moduleKey、当前 handler、链路信息等）
type HandlerContext struct {
	ctx context.Context

	Protocol Protocol
	RouteKey string

	Container *di.Container
	Request   *Request
	Response  *Response

	Metadata map[string]interface{}
	mu       sync.RWMutex
}

// Request/Response 是统一协议抽象，字段尽量保持“可通用 + 可回填”：
// - Raw 用于保留原生对象（例如 http.Request / grpc.ServerStream），供 adapter 或自定义 binding 使用
// - Params/Query/Headers 同时提供单值与多值两种形态，适配不同协议/网关行为
// Request 是各协议请求对象在 runtime 中的统一抽象。
type Request struct {
	Method       string
	Path         string
	Headers      map[string]string
	HeadersMulti map[string][]string
	Query        map[string]string
	QueryMulti   map[string][]string
	Params       map[string]string
	Body         interface{}
	BodyBytes    []byte
	Raw          interface{}
}

// Response 是各协议响应对象在 runtime 中的统一抽象。
type Response struct {
	Status  int
	Headers map[string]string
	Body    interface{}
	Raw     interface{}
}

// NewHandlerContext 创建一个新的 HandlerContext。
// 适合较长生命周期或不关心对象池复用的场景。
func NewHandlerContext(protocol Protocol) *HandlerContext {
	return &HandlerContext{
		ctx:      context.Background(),
		Protocol: protocol,
		Metadata: make(map[string]interface{}),
		Request: &Request{
			Headers:      make(map[string]string),
			HeadersMulti: make(map[string][]string),
			Query:        make(map[string]string),
			QueryMulti:   make(map[string][]string),
			Params:       make(map[string]string),
		},
		Response: &Response{
			Status:  200,
			Headers: make(map[string]string),
		},
	}
}

// Context 返回底层 context.Context（实现 context.Context 接口的一部分）。
//
// 该方法与 GetContext 等价，保留是为了兼容不同调用习惯。
func (h *HandlerContext) Context() context.Context {
	return h.ctx
}

// GetContext 返回底层 context.Context。
//
// AOP/adapter/业务代码可用它获取 deadline/cancel/value 等标准能力。
func (h *HandlerContext) GetContext() context.Context {
	return h.ctx
}

// WithContext 替换当前 HandlerContext 使用的底层 context.Context，并返回自身便于链式调用。
func (h *HandlerContext) WithContext(ctx context.Context) *HandlerContext {
	h.ctx = ctx
	return h
}

// WithTimeout 在当前 ctx 基础上设置超时；注意返回 cancel 必须由调用方负责调用。
func (h *HandlerContext) WithTimeout(timeout time.Duration) (*HandlerContext, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(h.ctx, timeout)
	h.ctx = ctx
	return h, cancel
}

// Set 写入一个元数据键值对（线程安全）。
//
// Metadata 用于 runtime 内部或 integration 透传信息，例如：
// - "kernel.moduleKey"：模块标识
// - "kernel.handler"：当前匹配到的 handler
func (h *HandlerContext) Set(key string, value interface{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Metadata[key] = value
}

// Get 读取一个元数据键值对（线程安全）。
func (h *HandlerContext) Get(key string) (interface{}, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	val, ok := h.Metadata[key]
	return val, ok
}

// Require 获取指定 key 的元数据；若不存在则返回 error（不 panic）。
func (h *HandlerContext) Require(key string) (interface{}, error) {
	val, ok := h.Get(key)
	if !ok {
		return nil, fmt.Errorf("key not found in context: %s", key)
	}
	return val, nil
}

// MustGet 获取指定 key 的元数据；若不存在则 panic。
//
// 适合框架内部“强约束存在”的字段；业务侧建议用 Require/Get 避免 panic。
func (h *HandlerContext) MustGet(key string) interface{} {
	val, err := h.Require(key)
	if err != nil {
		panic(err)
	}
	return val
}

// Deadline 返回底层 ctx 的 deadline（实现 context.Context 接口的一部分）。
func (h *HandlerContext) Deadline() (time.Time, bool) {
	return h.ctx.Deadline()
}

// Done 返回底层 ctx 的 Done channel（实现 context.Context 接口的一部分）。
func (h *HandlerContext) Done() <-chan struct{} {
	return h.ctx.Done()
}

// Err 返回底层 ctx 的错误（实现 context.Context 接口的一部分）。
func (h *HandlerContext) Err() error {
	return h.ctx.Err()
}

// Value 实现 context.Context 的 Value 查找语义，并额外支持从 Metadata 中按 string key 查找。
//
// 规则：
// - 如果 key 是 string，会先尝试从 HandlerContext.Metadata 读取（允许框架内部快速透传）
// - 否则回退到底层 ctx.Value(key)
func (h *HandlerContext) Value(key interface{}) interface{} {
	if strKey, ok := key.(string); ok {
		if val, exists := h.Get(strKey); exists {
			return val
		}
	}
	return h.ctx.Value(key)
}

// handlerContextPool 用于复用 HandlerContext，减少高频请求下的对象分配。
var handlerContextPool = sync.Pool{
	New: func() interface{} {
		return &HandlerContext{
			Metadata: make(map[string]interface{}),
			Request: &Request{
				Headers:      make(map[string]string),
				HeadersMulti: make(map[string][]string),
				Query:        make(map[string]string),
				QueryMulti:   make(map[string][]string),
				Params:       make(map[string]string),
			},
			Response: &Response{
				Status:  200,
				Headers: make(map[string]string),
			},
		}
	},
}

// AcquireHandlerContext 从对象池获取一个 HandlerContext，用于高频请求场景减少分配。
//
// 使用约定：
// - 仅用于“短生命周期”的请求处理；用完必须调用 ReleaseHandlerContext
// - 所有可变字段都必须在 Acquire/Release 时正确 reset，否则会出现跨请求串数据
func AcquireHandlerContext(protocol Protocol) *HandlerContext {
	hc := handlerContextPool.Get().(*HandlerContext)
	hc.ctx = context.Background()
	hc.Protocol = protocol
	hc.RouteKey = ""
	hc.Container = nil
	// 这里只重置“频繁变化的大字段”，底层 map 保持复用，减少高频请求下的分配抖动。
	hc.Request.Body = nil
	hc.Request.BodyBytes = nil
	hc.Request.Raw = nil
	hc.Response.Status = 200
	hc.Response.Body = nil
	hc.Response.Raw = nil
	return hc
}

// ReleaseHandlerContext 将 HandlerContext 归还对象池。
// 注意：这里会清理 map 内容，但不会把 map 置 nil，目的是复用底层容量减少分配。
func ReleaseHandlerContext(hc *HandlerContext) {
	if hc == nil {
		return
	}
	hc.ctx = nil
	hc.RouteKey = ""
	hc.Container = nil
	hc.Request.Method = ""
	hc.Request.Path = ""
	hc.Request.Body = nil
	hc.Request.BodyBytes = nil
	hc.Request.Raw = nil
	hc.Response.Status = 200
	hc.Response.Body = nil
	hc.Response.Raw = nil
	for k := range hc.Metadata {
		delete(hc.Metadata, k)
	}
	for k := range hc.Request.Headers {
		delete(hc.Request.Headers, k)
	}
	for k := range hc.Request.HeadersMulti {
		delete(hc.Request.HeadersMulti, k)
	}
	for k := range hc.Request.Query {
		delete(hc.Request.Query, k)
	}
	for k := range hc.Request.QueryMulti {
		delete(hc.Request.QueryMulti, k)
	}
	for k := range hc.Request.Params {
		delete(hc.Request.Params, k)
	}
	for k := range hc.Response.Headers {
		delete(hc.Response.Headers, k)
	}
	handlerContextPool.Put(hc)
}
