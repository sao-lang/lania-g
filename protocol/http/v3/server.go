// server.go 实现 HTTP adapter 的服务器启动与关闭逻辑。
package http

import (
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	httpbinding "github.com/sao-lang/lania-g/protocol/http/v3/binding"
	httpprotocol "github.com/sao-lang/lania-g/protocol/http/v3/protocol"
)

// ServeHTTP 实现 `http.Handler`，负责把 `net/http` 请求转发到框架 runtime handler。
//
// 整体顺序是：
// - 处理 adapter 级 helmet/cors
// - 处理 basePath 与 mounted 子 handler
// - 把原始请求投影成 runtime.HandlerContext
// - 跑 middleware 链，再执行 runtime handler
// - 若业务未自行写响应，则走统一的 writeHTTPResponse
func (a *Adapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if a.host == nil {
		http.Error(w, "http adapter not mounted", http.StatusInternalServerError)
		return
	}
	if a.helmet != nil {
		applyHelmet(w, r, a.helmet)
	}
	if a.cors != nil {
		if handled := applyCORS(w, r, a.cors); handled {
			return
		}
	}

	path, ok := a.requestPath(r)
	if !ok {
		a.nextHandler.ServeHTTP(w, r)
		return
	}
	if mh := a.matchMounted(path); mh != nil {
		// mounted handler 看到的是去掉 basePath 之后的子路径，保持子应用视角一致。
		mh.ServeHTTP(w, cloneRequestWithPath(r, path))
		return
	}

	ctx, httpCtx, err := a.buildHandlerContext(w, r, path)
	if err != nil {
		if errors.Is(err, errRequestBodyTooLarge) {
			writeRequestError(w, r, http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		writeRequestError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	defer runtime.ReleaseHandlerContext(ctx)

	result, executed, nextDownstreamCalled, execErr := a.executeMiddlewareChain(ctx, httpCtx, w, r, path)
	if execErr != nil {
		if nextDownstreamCalled {
			return
		}
		writeHTTPError(w, r, ctx, execErr)
		return
	}
	if nextDownstreamCalled || !executed {
		return
	}
	if writtenAny, ok := ctx.Get(httpbinding.MetadataKeyWritten); ok {
		if written, ok := writtenAny.(bool); ok && written {
			// 某些中间件/handler 会自己直接写回响应，这时不要再重复写框架默认响应。
			return
		}
	}

	handler, _ := a.host.Runtime().GetRouter().Get(ctx.RouteKey)
	writeHTTPResponse(w, r, handler, ctx, result, a.renderer)
}

func (a *Adapter) requestPath(r *http.Request) (string, bool) {
	path := r.URL.Path
	base := normalizeBasePath(a.basePath)
	if base == "" {
		return path, true
	}
	if path == base {
		return "/", true
	}
	if strings.HasPrefix(path, base+"/") {
		// 进入 mounted adapter 后，把 basePath 从请求路径里剥掉，
		// 后续路由匹配统一基于“子应用内部路径”进行。
		path = strings.TrimPrefix(path, base)
		if path == "" {
			path = "/"
		}
		return path, true
	}
	return "", false
}

func (a *Adapter) buildHandlerContext(w http.ResponseWriter, r *http.Request, path string) (*runtime.HandlerContext, *httpbinding.HttpContext, error) {
	ctx := runtime.AcquireHandlerContext(httpprotocol.Protocol)
	ctx.WithContext(r.Context())
	ctx.Request.Raw = r
	ctx.Response.Raw = w
	ctx.Request.Method = r.Method
	ctx.Request.Path = path
	copyHeaderValues(ctx.Request.Headers, ctx.Request.HeadersMulti, r.Header)
	ctx.Request.Headers["Host"] = r.Host
	// 空 query 时完全跳过：HandlerContext 是池化复用的，Query/QueryMulti 已经是清空后的 map，
	// 再构造一次只会白白覆盖成两个新 map（并丢掉池化收益）。
	if r.URL.RawQuery != "" {
		ctx.Request.Query, ctx.Request.QueryMulti = queryValues(r.URL.Query())
	}

	if shouldReadBody(r) {
		// 这里在入口一次性受限读取 body，后续 binding 层统一消费缓存的 bodyBytes。
		bodyBytes, err := readBodyLimited(r, a.maxBodyBytes)
		if err != nil {
			runtime.ReleaseHandlerContext(ctx)
			return nil, nil, err
		}
		ctx.Request.BodyBytes = bodyBytes
	}

	if files := parseUploadedFiles(r); len(files) > 0 {
		ctx.Set(httpbinding.MetadataKeyFiles, files)
	}
	if form := parseFormValues(r); len(form) > 0 {
		ctx.Set(httpbinding.MetadataKeyForm, form)
	}

	httpCtx, _ := httpbinding.NewHttpContext(ctx)
	if httpCtx != nil {
		// 这些 metadata 是 binding/http resolver 的主要输入来源。
		ctx.Set(httpbinding.MetadataKeyContext, httpCtx)
		ctx.Set(httpbinding.MetadataKeyRenderer, a.renderer)
		if a.validator != nil {
			ctx.Set(httpbinding.MetadataKeyValidator, a.validator)
		}
	}
	return ctx, httpCtx, nil
}

// httpChainState 保存一次请求在 preRouting 链上的可变状态。
//
// 这些字段都会被 next 闭包捕获。集中到同一个结构体后，整个请求只逃逸这一个对象，
// 而不是让 result/execErr/index 各自逃逸（原来 4 次分配 → 现在 1 次）。
type httpChainState struct {
	result               any
	execErr              error
	executed             bool
	nextDownstreamCalled bool
	downstreamOnce       sync.Once
	index                int
}

func (a *Adapter) executeMiddlewareChain(ctx *runtime.HandlerContext, httpCtx *httpbinding.HttpContext, w http.ResponseWriter, r *http.Request, path string) (any, bool, bool, error) {
	state := &httpChainState{index: -1}

	nextFn := func() error {
		if httpCtx != nil && httpCtx.Aborted() {
			// 兼容 gin/echo 风格的“中止链路”语义：一旦 abort，后续 middleware/handler 都不再继续。
			return nil
		}
		state.index++
		if state.index < len(a.preRouting) {
			return a.preRouting[state.index](httpCtx)
		}
		if !state.executed {
			// 首次穿过 middleware 链时进入 runtime.Execute；
			// 后续再调用 next() 则意味着业务想把请求继续交给下游 http.Handler。
			state.executed = true
			state.result, state.execErr = a.host.Runtime().Execute(ctx)
			return state.execErr
		}
		state.downstreamOnce.Do(func() {
			state.nextDownstreamCalled = true
			if a.nextHandler != nil {
				a.nextHandler.ServeHTTP(w, cloneRequestWithPath(r, path))
			}
		})
		return nil
	}

	// MetadataKeyNext 只接受 func() error，直接存 nextFn 即可，无需再包一层闭包。
	ctx.Set(httpbinding.MetadataKeyNext, nextFn)
	if err := nextFn(); err != nil {
		return state.result, state.executed, state.nextDownstreamCalled, err
	}
	if state.execErr != nil {
		return state.result, state.executed, state.nextDownstreamCalled, state.execErr
	}
	return state.result, state.executed, state.nextDownstreamCalled, nil
}
