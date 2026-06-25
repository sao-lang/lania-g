// dsl.go 提供 HTTP adapter 的声明式注册 DSL。
package http

import (
	"fmt"
	"maps"

	coreadapter "github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// 一个最小示例：
//
//	builder := http.Controller("/users", &UserController{})
//	builder.UseGuards(AuthGuard{})
//	builder.Get("/:id", (*UserController).GetByID).
//		Summary("根据 ID 获取用户")
//	builder.Post("/", (*UserController).Create)
//	builder.Build()
//
// 小白阅读时可以按这个顺序理解：
// - Controller(...) 先定义 controller 级别的公共信息，例如 prefix 和共享 AOP
// - Get/Post/... 表示开始声明一条具体路由
// - RouteBuilder 上的 UseGuards/UsePipes/... 只作用于当前这条路由
// - Build() 会把收集到的声明写入 registry，等待后续编译阶段读取

// Controller 提供一个接近 v2 风格的全局 DSL 兼容入口：
//
//	builder := http.Controller("/users", controller)
//
// 它会把声明写入 `core/registry.Global()`。
// 新业务代码更推荐通过 mounted adapter 暴露的 `adapter.API()` 在应用实例上注册声明。
func Controller(prefix string, controller any) *ControllerBuilder {
	return (&API{reg: registry.Global(), fallbackSource: "http.Controller"}).Controller(prefix, controller)
}

// ControllerBuilder 用于声明某个 HTTP controller 及其共享配置。
//
// 它承载 controller 级别的 prefix、共享 AOP 和路由列表，
// 最终通过 `Build/BuildE` 把收集到的路由定义写入 registry。
type ControllerBuilder struct {
	prefix         string
	controller     any
	routes         []*RouteBuilder
	guards         []any
	interceptors   []any
	middlewares    []any
	pipes          []any
	paramPipes     map[int][]any
	filters        []any
	registry       *registry.Registry
	fallbackSource string
	err            error
}

// RouteBuilder 用于声明一条具体 HTTP 路由。
//
// 它会继承所属 ControllerBuilder 的公共配置，
// 并允许再对当前路由追加局部的 AOP、文档和响应行为。
type RouteBuilder struct {
	controllerBuilder *ControllerBuilder
	method            Method
	path              string
	handler           any
	methodName        string
	guards            []any
	interceptors      []any
	middlewares       []any
	pipes             []any
	paramPipes        map[int][]any
	filters           []any
	statusCode        int
	headers           map[string]string
	doc               *RouteDoc
	redirect          *RedirectConfig
	render            string
	sealed            bool
}

// RouteTerminalBuilder 表示当前路由已经被 StatusCode/Redirect/Render
// 这类终结方法收口。从这一刻起，这条路由视为完整，
// 链式调用只允许开始下一条路由，或者直接 Build()。
type RouteTerminalBuilder struct {
	controllerBuilder *ControllerBuilder
}

func newControllerBuilder(prefix string, controller any, reg *registry.Registry, fallbackSource string) *ControllerBuilder {
	if reg == nil {
		// nil registry 仍回退到全局 registry，继续兼容 v2/v早期 v3 的全局 DSL 用法。
		reg = registry.Global()
	}
	return &ControllerBuilder{
		prefix:         prefix,
		controller:     controller,
		routes:         make([]*RouteBuilder, 0),
		paramPipes:     make(map[int][]any),
		registry:       reg,
		fallbackSource: fallbackSource,
	}
}

// UseParamPipes 在 controller 级别注册参数级 Pipe。
//
// 它必须在声明第一条路由前调用。
// 这里配置的 Pipe 会被后续路由继承，除非某条路由自己追加更多参数级 Pipe。
func (b *ControllerBuilder) UseParamPipes(paramIndex int, pipes ...any) *ControllerBuilder {
	b.assertCanConfigureControllerScope()
	if b.paramPipes[paramIndex] == nil {
		b.paramPipes[paramIndex] = make([]any, 0)
	}
	b.paramPipes[paramIndex] = append(b.paramPipes[paramIndex], pipes...)
	return b
}

// UseGuards 在 controller 级别追加守卫，后续声明的所有路由都会继承。
func (b *ControllerBuilder) UseGuards(items ...any) *ControllerBuilder {
	b.assertCanConfigureControllerScope()
	b.guards = append(b.guards, items...)
	return b
}

// UseInterceptors 在 controller 级别追加拦截器，后续声明的所有路由都会继承。
func (b *ControllerBuilder) UseInterceptors(items ...any) *ControllerBuilder {
	b.assertCanConfigureControllerScope()
	b.interceptors = append(b.interceptors, items...)
	return b
}

// UseMiddlewares 在 controller 级别追加中间件，后续声明的所有路由都会继承。
func (b *ControllerBuilder) UseMiddlewares(items ...any) *ControllerBuilder {
	b.assertCanConfigureControllerScope()
	b.middlewares = append(b.middlewares, items...)
	return b
}

// UsePipes 在 controller 级别追加 Pipe，后续声明的所有路由都会继承。
func (b *ControllerBuilder) UsePipes(items ...any) *ControllerBuilder {
	b.assertCanConfigureControllerScope()
	b.pipes = append(b.pipes, items...)
	return b
}

// UseFilters 在 controller 级别追加异常过滤器，后续声明的所有路由都会继承。
func (b *ControllerBuilder) UseFilters(items ...any) *ControllerBuilder {
	b.assertCanConfigureControllerScope()
	b.filters = append(b.filters, items...)
	return b
}

// Get 声明一条 `GET` 路由。
func (b *ControllerBuilder) Get(path string, handler any) *RouteBuilder {
	return b.addRoute(GET, path, handler)
}

// Post 声明一条 `POST` 路由。
func (b *ControllerBuilder) Post(path string, handler any) *RouteBuilder {
	return b.addRoute(POST, path, handler)
}

// Put 声明一条 `PUT` 路由。
func (b *ControllerBuilder) Put(path string, handler any) *RouteBuilder {
	return b.addRoute(PUT, path, handler)
}

// Delete 声明一条 `DELETE` 路由。
func (b *ControllerBuilder) Delete(path string, handler any) *RouteBuilder {
	return b.addRoute(DELETE, path, handler)
}

// Patch 声明一条 `PATCH` 路由。
func (b *ControllerBuilder) Patch(path string, handler any) *RouteBuilder {
	return b.addRoute(PATCH, path, handler)
}

// Head 声明一条 `HEAD` 路由。
func (b *ControllerBuilder) Head(path string, handler any) *RouteBuilder {
	return b.addRoute(HEAD, path, handler)
}

// Options 声明一条 `OPTIONS` 路由。
func (b *ControllerBuilder) Options(path string, handler any) *RouteBuilder {
	return b.addRoute(OPTIONS, path, handler)
}

// All 声明一条对所有 HTTP 方法生效的路由。
func (b *ControllerBuilder) All(path string, handler any) *RouteBuilder {
	return b.addRoute(ALL, path, handler)
}

func (b *ControllerBuilder) addRoute(method Method, path string, handler any) *RouteBuilder {
	rb := &RouteBuilder{
		controllerBuilder: b,
		method:            method,
		path:              path,
		handler:           handler,
		// DSL 里允许直接传 bound method / method expression，
		// 这里尽早把它解析成 methodName，后续编译阶段只处理字符串方法名。
		methodName:   coreadapter.FindMethodName(b.controller, handler),
		guards:       make([]any, 0),
		interceptors: make([]any, 0),
		middlewares:  make([]any, 0),
		pipes:        make([]any, 0),
		paramPipes:   make(map[int][]any),
		filters:      make([]any, 0),
		headers:      make(map[string]string),
		doc:          &RouteDoc{ErrorResponses: make(map[int]string)},
	}
	b.routes = append(b.routes, rb)
	return rb
}

// Build 返回当前 controller 收集到的路由定义；忽略构建错误。
func (b *ControllerBuilder) Build() []*RouteDefinition {
	defs, _ := b.BuildE()
	return defs
}

// BuildE 完成当前 controller 的声明收集，并把所有路由注册进 registry。
//
// 它不会启动 HTTP 服务，只是产出路由定义，供 `application.Application`
// 在后续编译阶段读取和安装。
func (b *ControllerBuilder) BuildE() ([]*RouteDefinition, error) {
	if b.err != nil {
		return nil, b.err
	}
	defs := make([]*RouteDefinition, 0, len(b.routes))
	for _, rb := range b.routes {
		defs = append(defs, rb.build(b))
		rb.seal()
	}
	items := make([]any, 0, len(defs))
	for _, def := range defs {
		items = append(items, def)
	}
	if b.fallbackSource != "" {
		b.registry.RecordFallbackUsage(b.fallbackSource)
	}
	b.registry.RegisterDecl(AdapterID, "routes", items...)
	return defs, nil
}

// Err 返回链式声明过程中记录下来的 controller 级 DSL 使用错误。
//
// 最常见的情况是：第一条路由已经声明之后，才去调用 controller 级的 Use* 方法。
func (b *ControllerBuilder) Err() error { return b.err }

// UseGuards 为当前路由追加 guards。
func (rb *RouteBuilder) UseGuards(items ...any) *RouteBuilder {
	if rb.sealed {
		return rb
	}
	rb.guards = append(rb.guards, items...)
	return rb
}

// UseInterceptors 为当前路由追加 interceptors。
func (rb *RouteBuilder) UseInterceptors(items ...any) *RouteBuilder {
	if rb.sealed {
		return rb
	}
	rb.interceptors = append(rb.interceptors, items...)
	return rb
}

// UseMiddlewares 为当前路由追加 middlewares。
func (rb *RouteBuilder) UseMiddlewares(items ...any) *RouteBuilder {
	if rb.sealed {
		return rb
	}
	rb.middlewares = append(rb.middlewares, items...)
	return rb
}

// UsePipes 为当前路由追加 pipes。
func (rb *RouteBuilder) UsePipes(items ...any) *RouteBuilder {
	if rb.sealed {
		return rb
	}
	rb.pipes = append(rb.pipes, items...)
	return rb
}

// UseParamPipes 为指定参数位置追加 pipes。
func (rb *RouteBuilder) UseParamPipes(paramIndex int, items ...any) *RouteBuilder {
	if rb.sealed {
		return rb
	}
	if rb.paramPipes[paramIndex] == nil {
		rb.paramPipes[paramIndex] = make([]any, 0)
	}
	rb.paramPipes[paramIndex] = append(rb.paramPipes[paramIndex], items...)
	return rb
}

// UseFilters 为当前路由追加 filters。
func (rb *RouteBuilder) UseFilters(items ...any) *RouteBuilder {
	if rb.sealed {
		return rb
	}
	rb.filters = append(rb.filters, items...)
	return rb
}

// StatusCode 把当前路由标记为终结态，并设置固定的成功状态码。
//
// 一旦调用 StatusCode、Redirect、Render 这类终结方法，
// 当前路由就视为声明完成；后续链式调用只能开始下一条路由，
// 或直接在 controller 上调用 Build。
func (rb *RouteBuilder) StatusCode(code int) *RouteTerminalBuilder {
	rb.statusCode = code
	rb.seal()
	return &RouteTerminalBuilder{controllerBuilder: rb.controllerBuilder}
}

// Header 为当前路由追加一个响应头。
func (rb *RouteBuilder) Header(key, value string) *RouteBuilder {
	// header 配置是纯声明收集；真正写响应头发生在 adapter 运行时写回响应时。
	rb.headers[key] = value
	return rb
}

// Summary 设置当前路由的文档摘要。
func (rb *RouteBuilder) Summary(summary string) *RouteBuilder {
	if rb.doc != nil {
		rb.doc.Summary = summary
	}
	return rb
}

// Description 设置当前路由的文档描述。
func (rb *RouteBuilder) Description(description string) *RouteBuilder {
	if rb.doc != nil {
		rb.doc.Description = description
	}
	return rb
}

// Tags 为当前路由追加文档标签。
func (rb *RouteBuilder) Tags(tags ...string) *RouteBuilder {
	if rb.doc != nil {
		rb.doc.Tags = append(rb.doc.Tags, tags...)
	}
	return rb
}

// Security 为当前路由追加文档安全要求。
func (rb *RouteBuilder) Security(name string, scopes ...string) *RouteBuilder {
	if rb.doc != nil && name != "" {
		rb.doc.Security = append(rb.doc.Security, SecurityRequirement{
			Name:   name,
			Scopes: append([]string{}, scopes...),
		})
	}
	return rb
}

// ResponseBody 设置当前路由的响应体示例类型。
func (rb *RouteBuilder) ResponseBody(example any) *RouteBuilder {
	if rb.doc != nil {
		rb.doc.ResponseType = example
		rb.doc.ResponseField = ""
	}
	return rb
}

// ResponseEnvelope 设置当前路由的响应包裹结构与字段名。
func (rb *RouteBuilder) ResponseEnvelope(envelope any, field string) *RouteBuilder {
	if rb.doc != nil {
		rb.doc.ResponseType = envelope
		rb.doc.ResponseField = field
	}
	return rb
}

// ErrorResponse 为当前路由追加一个错误响应文档项。
func (rb *RouteBuilder) ErrorResponse(status int, description string) *RouteBuilder {
	if rb.doc != nil && status > 0 && description != "" {
		if rb.doc.ErrorResponses == nil {
			rb.doc.ErrorResponses = make(map[int]string)
		}
		rb.doc.ErrorResponses[status] = description
	}
	return rb
}

// HideFromDocs 把当前路由标记为不出现在文档中。
func (rb *RouteBuilder) HideFromDocs() *RouteBuilder {
	if rb.doc != nil {
		rb.doc.Hidden = true
	}
	return rb
}

// Headers 批量追加当前路由的响应头。
func (rb *RouteBuilder) Headers(headers map[string]string) *RouteBuilder {
	maps.Copy(rb.headers, headers)
	return rb
}

// Redirect 把当前路由标记为终结态，并配置一个 HTTP 重定向响应。
func (rb *RouteBuilder) Redirect(url string, status ...int) *RouteTerminalBuilder {
	s := 302
	if len(status) > 0 {
		s = status[0]
	}
	rb.redirect = &RedirectConfig{URL: url, Status: s}
	rb.seal()
	return &RouteTerminalBuilder{controllerBuilder: rb.controllerBuilder}
}

// Render 把当前路由标记为终结态，并配置要渲染的模板名。
func (rb *RouteBuilder) Render(template string) *RouteTerminalBuilder {
	rb.render = template
	rb.seal()
	return &RouteTerminalBuilder{controllerBuilder: rb.controllerBuilder}
}

// Get 在当前 controller 下继续声明下一条 `GET` 路由。
func (rb *RouteBuilder) Get(path string, handler any) *RouteBuilder {
	return rb.controllerBuilder.Get(path, handler)
}

// Post 在当前 controller 下继续声明下一条 `POST` 路由。
func (rb *RouteBuilder) Post(path string, handler any) *RouteBuilder {
	return rb.controllerBuilder.Post(path, handler)
}

// Put 在当前 controller 下继续声明下一条 `PUT` 路由。
func (rb *RouteBuilder) Put(path string, handler any) *RouteBuilder {
	return rb.controllerBuilder.Put(path, handler)
}

// Delete 在当前 controller 下继续声明下一条 `DELETE` 路由。
func (rb *RouteBuilder) Delete(path string, handler any) *RouteBuilder {
	return rb.controllerBuilder.Delete(path, handler)
}

// Patch 在当前 controller 下继续声明下一条 `PATCH` 路由。
func (rb *RouteBuilder) Patch(path string, handler any) *RouteBuilder {
	return rb.controllerBuilder.Patch(path, handler)
}

// Head 在当前 controller 下继续声明下一条 `HEAD` 路由。
func (rb *RouteBuilder) Head(path string, handler any) *RouteBuilder {
	return rb.controllerBuilder.Head(path, handler)
}

// Options 在当前 controller 下继续声明下一条 `OPTIONS` 路由。
func (rb *RouteBuilder) Options(path string, handler any) *RouteBuilder {
	return rb.controllerBuilder.Options(path, handler)
}

// All 在当前 controller 下继续声明下一条 `ALL` 路由。
func (rb *RouteBuilder) All(path string, handler any) *RouteBuilder {
	return rb.controllerBuilder.All(path, handler)
}

// Build 返回当前 controller 已声明的路由定义。
func (rb *RouteBuilder) Build() []*RouteDefinition { return rb.controllerBuilder.Build() }

// Get 在终结态之后开始声明下一条 `GET` 路由。
func (tb *RouteTerminalBuilder) Get(path string, handler any) *RouteBuilder {
	return tb.controllerBuilder.Get(path, handler)
}

// Post 在终结态之后开始声明下一条 `POST` 路由。
func (tb *RouteTerminalBuilder) Post(path string, handler any) *RouteBuilder {
	return tb.controllerBuilder.Post(path, handler)
}

// Put 在终结态之后开始声明下一条 `PUT` 路由。
func (tb *RouteTerminalBuilder) Put(path string, handler any) *RouteBuilder {
	return tb.controllerBuilder.Put(path, handler)
}

// Delete 在终结态之后开始声明下一条 `DELETE` 路由。
func (tb *RouteTerminalBuilder) Delete(path string, handler any) *RouteBuilder {
	return tb.controllerBuilder.Delete(path, handler)
}

// Patch 在终结态之后开始声明下一条 `PATCH` 路由。
func (tb *RouteTerminalBuilder) Patch(path string, handler any) *RouteBuilder {
	return tb.controllerBuilder.Patch(path, handler)
}

// Head 在终结态之后开始声明下一条 `HEAD` 路由。
func (tb *RouteTerminalBuilder) Head(path string, handler any) *RouteBuilder {
	return tb.controllerBuilder.Head(path, handler)
}

// Options 在终结态之后开始声明下一条 `OPTIONS` 路由。
func (tb *RouteTerminalBuilder) Options(path string, handler any) *RouteBuilder {
	return tb.controllerBuilder.Options(path, handler)
}

// All 在终结态之后开始声明下一条 `ALL` 路由。
func (tb *RouteTerminalBuilder) All(path string, handler any) *RouteBuilder {
	return tb.controllerBuilder.All(path, handler)
}

// Build 返回当前 controller 已声明的路由定义。
func (tb *RouteTerminalBuilder) Build() []*RouteDefinition { return tb.controllerBuilder.Build() }

func (rb *RouteBuilder) seal() { rb.sealed = true }

// assertCanConfigureControllerScope 用于阻止在第一条路由声明之后，
// 再去修改 controller 级别的共享配置。
//
// 这样可以保证 DSL 的行为稳定可预测：共享配置必须在路由声明开始前确定。
func (b *ControllerBuilder) assertCanConfigureControllerScope() {
	if len(b.routes) > 0 && b.err == nil {
		b.err = fmt.Errorf("http.ControllerBuilder: controller-level Use* APIs must be called before declaring the first route")
	}
}

func (rb *RouteBuilder) build(cb *ControllerBuilder) *RouteDefinition {
	return &RouteDefinition{
		Method:       rb.method,
		Path:         cb.prefix + rb.path,
		Controller:   cb.controller,
		MethodName:   rb.methodName,
		Guards:       append(append([]any{}, cb.guards...), rb.guards...),
		Interceptors: append(append([]any{}, cb.interceptors...), rb.interceptors...),
		Middlewares:  append(append([]any{}, cb.middlewares...), rb.middlewares...),
		Pipes:        append(append([]any{}, cb.pipes...), rb.pipes...),
		// 路由级参数 pipe 追加在 controller 级配置之后，形成最终执行顺序。
		ParamPipes: coreadapter.MergeParamPipes(cb.paramPipes, rb.paramPipes),
		Filters:    append(append([]any{}, cb.filters...), rb.filters...),
		StatusCode: rb.statusCode,
		Headers:    copyHeaders(rb.headers),
		Redirect:   rb.redirect,
		Render:     rb.render,
		Doc:        copyRouteDoc(rb.doc),
	}
}

func copyHeaders(src map[string]string) map[string]string {
	out := make(map[string]string, len(src))
	maps.Copy(out, src)
	return out
}

// copyRouteDoc 做声明期快照，避免多个路由 builder 共享底层切片/map。
func copyRouteDoc(src *RouteDoc) *RouteDoc {
	if src == nil {
		return nil
	}
	out := *src
	out.Tags = append([]string{}, src.Tags...)
	if len(src.Security) > 0 {
		out.Security = make([]SecurityRequirement, 0, len(src.Security))
		for _, item := range src.Security {
			out.Security = append(out.Security, SecurityRequirement{
				Name:   item.Name,
				Scopes: append([]string{}, item.Scopes...),
			})
		}
	}
	if len(src.ErrorResponses) > 0 {
		out.ErrorResponses = make(map[int]string, len(src.ErrorResponses))
		maps.Copy(out.ErrorResponses, src.ErrorResponses)
	}
	return &out
}
