// routes.go 定义 HTTP 路由声明。
//
// 声明方式只有一种：控制器实现 Controller 接口，在所属模块内完成注册。
//
//	func (c *UserController) Register(r *http.Routes) {
//		r.Prefix("/users").
//			UseGuards(AuthGuard{}).
//			Get("/:id", c.Get).
//				Summary("Get User").
//				Tags("users").
//			Post("", c.Create).
//				Status(201)
//	}
//
// 规则只有三条：
//  1. 第一个动词方法之前的 Use*/Prefix 作用于整个控制器
//  2. 动词方法开始一条新路由
//  3. 其余修饰方法作用于“当前路由”；出现在任何动词之前视为声明错误
package http

import (
	"fmt"
	"maps"
	"strings"

	coreadapter "github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// Controller 由需要声明 HTTP 路由的控制器实现。
//
// 模块加载完成后，协议层会为每个实现了该接口的控制器创建一份 Routes，
// 并调用 Register。控制器因此始终归属于它所在的模块。
type Controller interface {
	Declare(*Routes)
}

// Routes 是唯一的路由声明类型。
//
// 作用域方法（Prefix/Use*）与路由修饰方法（Summary/Status/...）都返回同一个类型，
// 因此不存在“终结态”，也不需要手动 Build。
type Routes struct {
	reg        *registry.Registry
	prefix     string
	controller any
	errs       []string

	scope  aopScope
	drafts []*routeDraft
	cur    *routeDraft
}

type aopScope struct {
	guards       []aop.Guard
	interceptors []aop.Interceptor
	middlewares  []aop.Middleware
	pipes        []aop.Pipe
	filters      []aop.ExceptionFilter
	paramPipes   map[int][]aop.Pipe
}

type routeDraft struct {
	method     Method
	path       string
	handler    any
	methodName string
	aop        aopScope
	statusCode int
	headers    map[string]string
	redirect   *RedirectConfig
	render     string
	doc        *RouteDoc
}

// NewRoutes 创建一个绑定到 reg 的声明收集器。
//
// 正常装配由 adapter 在 Declare 阶段构造；测试或自定义装载器可直接使用。
func NewRoutes(reg *registry.Registry, controller any) *Routes {
	return &Routes{reg: reg, controller: controller}
}

// Prefix 声明控制器级路径前缀，等价于 NestJS 的 @Controller('users')。
//
// 只能在第一条路由之前调用。
func (r *Routes) Prefix(prefix string) *Routes {
	if len(r.drafts) > 0 || r.cur != nil {
		r.fail("Prefix() must be called before declaring the first route")
		return r
	}
	r.prefix = prefix
	return r
}

// UseGuards 声明守卫。
//
// 出现在第一条路由之前作用于整个控制器，之后作用于当前路由。
//
// 参数是 `aop.Guard` 接口：`aop.GuardFunc` 已实现它，因此具名函数类型可直接传入；
// 裸函数需要显式转换 `aop.GuardFunc(fn)`，这样签名错误在编译期就会暴露。
// 延迟构造器用 `aop.GuardFactory(ctor)` 包装。
func (r *Routes) UseGuards(guards ...aop.Guard) *Routes {
	s := r.scopeOrCurrent("UseGuards")
	s.guards = append(s.guards, guards...)
	return r
}

// UseInterceptors 声明拦截器。
func (r *Routes) UseInterceptors(interceptors ...aop.Interceptor) *Routes {
	s := r.scopeOrCurrent("UseInterceptors")
	s.interceptors = append(s.interceptors, interceptors...)
	return r
}

// UseMiddlewares 声明中间件。
func (r *Routes) UseMiddlewares(middlewares ...aop.Middleware) *Routes {
	s := r.scopeOrCurrent("UseMiddlewares")
	s.middlewares = append(s.middlewares, middlewares...)
	return r
}

// UsePipes 声明管道。
func (r *Routes) UsePipes(pipes ...aop.Pipe) *Routes {
	s := r.scopeOrCurrent("UsePipes")
	s.pipes = append(s.pipes, pipes...)
	return r
}

// UseFilters 声明异常过滤器。
func (r *Routes) UseFilters(filters ...aop.ExceptionFilter) *Routes {
	s := r.scopeOrCurrent("UseFilters")
	s.filters = append(s.filters, filters...)
	return r
}

// UseParamPipes 为指定参数位置声明管道。
func (r *Routes) UseParamPipes(paramIndex int, pipes ...aop.Pipe) *Routes {
	s := r.scopeOrCurrent("UseParamPipes")
	if s.paramPipes == nil {
		s.paramPipes = make(map[int][]aop.Pipe)
	}
	s.paramPipes[paramIndex] = append(s.paramPipes[paramIndex], pipes...)
	return r
}

// Get 开始声明一条 GET 路由。
func (r *Routes) Get(path string, handler any) *Routes { return r.verb(GET, path, handler) }

// Post 开始声明一条 POST 路由。
func (r *Routes) Post(path string, handler any) *Routes { return r.verb(POST, path, handler) }

// Put 开始声明一条 PUT 路由。
func (r *Routes) Put(path string, handler any) *Routes { return r.verb(PUT, path, handler) }

// Delete 开始声明一条 DELETE 路由。
func (r *Routes) Delete(path string, handler any) *Routes { return r.verb(DELETE, path, handler) }

// Patch 开始声明一条 PATCH 路由。
func (r *Routes) Patch(path string, handler any) *Routes { return r.verb(PATCH, path, handler) }

// Head 开始声明一条 HEAD 路由。
func (r *Routes) Head(path string, handler any) *Routes { return r.verb(HEAD, path, handler) }

// Options 开始声明一条 OPTIONS 路由。
func (r *Routes) Options(path string, handler any) *Routes { return r.verb(OPTIONS, path, handler) }

// All 开始声明一条对所有方法生效的路由。
func (r *Routes) All(path string, handler any) *Routes { return r.verb(ALL, path, handler) }

// Summary 设置当前路由的文档摘要。
func (r *Routes) Summary(summary string) *Routes { return r.docModifier("Summary", func(d *RouteDoc) { d.Summary = summary }) }

// Description 设置当前路由的文档描述。
func (r *Routes) Description(description string) *Routes {
	return r.docModifier("Description", func(d *RouteDoc) { d.Description = description })
}

// Tags 追加当前路由的文档标签。
func (r *Routes) Tags(tags ...string) *Routes {
	return r.docModifier("Tags", func(d *RouteDoc) { d.Tags = append(d.Tags, tags...) })
}

// Security 追加当前路由的文档安全要求。
//
// 它只影响 OpenAPI 生成，不参与鉴权；鉴权请使用 UseGuards。
func (r *Routes) Security(name string, scopes ...string) *Routes {
	if name == "" {
		return r
	}
	return r.docModifier("Security", func(d *RouteDoc) {
		d.Security = append(d.Security, SecurityRequirement{Name: name, Scopes: append([]string{}, scopes...)})
	})
}

// ResponseBody 设置当前路由的响应体示例类型。
func (r *Routes) ResponseBody(example any) *Routes {
	return r.docModifier("ResponseBody", func(d *RouteDoc) {
		d.ResponseType = example
		d.ResponseField = ""
	})
}

// ResponseEnvelope 设置当前路由的响应信封结构与数据字段名。
func (r *Routes) ResponseEnvelope(envelope any, field string) *Routes {
	return r.docModifier("ResponseEnvelope", func(d *RouteDoc) {
		d.ResponseType = envelope
		d.ResponseField = field
	})
}

// ErrorResponse 追加当前路由的错误响应文档项。
func (r *Routes) ErrorResponse(status int, description string) *Routes {
	return r.docModifier("ErrorResponse", func(d *RouteDoc) {
		if d.ErrorResponses == nil {
			d.ErrorResponses = make(map[int]string)
		}
		d.ErrorResponses[status] = description
	})
}

// HideFromDocs 把当前路由从文档中隐藏。
func (r *Routes) HideFromDocs() *Routes {
	return r.docModifier("HideFromDocs", func(d *RouteDoc) { d.Hidden = true })
}

// Status 设置当前路由的固定成功状态码。
func (r *Routes) Status(code int) *Routes {
	if cur := r.requireCurrent("Status"); cur != nil {
		cur.statusCode = code
	}
	return r
}

// Header 为当前路由追加一个响应头。
func (r *Routes) Header(key, value string) *Routes {
	if cur := r.requireCurrent("Header"); cur != nil {
		cur.headers[key] = value
	}
	return r
}

// Headers 批量追加当前路由的响应头。
func (r *Routes) Headers(headers map[string]string) *Routes {
	if cur := r.requireCurrent("Headers"); cur != nil {
		maps.Copy(cur.headers, headers)
	}
	return r
}

// Redirect 把当前路由声明为重定向响应。
func (r *Routes) Redirect(url string, status ...int) *Routes {
	if cur := r.requireCurrent("Redirect"); cur != nil {
		code := 302
		if len(status) > 0 {
			code = status[0]
		}
		cur.redirect = &RedirectConfig{URL: url, Status: code}
	}
	return r
}

// Render 把当前路由声明为模板渲染响应。
func (r *Routes) Render(template string) *Routes {
	if cur := r.requireCurrent("Render"); cur != nil {
		cur.render = template
	}
	return r
}

// Err 返回声明过程中累积的错误。
func (r *Routes) Err() error {
	if len(r.errs) == 0 {
		return nil
	}
	return fmt.Errorf("http routes for %T: %s", r.controller, strings.Join(r.errs, "; "))
}

// Defs 把当前声明转换为路由定义，返回前会完成校验。
func (r *Routes) Defs() ([]*RouteDefinition, error) {
	r.sealCurrent()
	if err := r.Err(); err != nil {
		return nil, err
	}
	defs := make([]*RouteDefinition, 0, len(r.drafts))
	for _, draft := range r.drafts {
		if draft == nil {
			continue
		}
		if draft.methodName == "" {
			r.fail(fmt.Sprintf("%s %s: handler does not belong to controller %T", draft.method, draft.path, r.controller))
			continue
		}
		defs = append(defs, r.build(draft))
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return defs, nil
}

func (r *Routes) verb(method Method, path string, handler any) *Routes {
	r.sealCurrent()
	r.cur = &routeDraft{
		method:     method,
		path:       path,
		handler:    handler,
		methodName: coreadapter.FindMethodName(r.controller, handler),
		headers:    make(map[string]string),
		doc:        &RouteDoc{ErrorResponses: make(map[int]string)},
	}
	return r
}

func (r *Routes) sealCurrent() {
	if r.cur == nil {
		return
	}
	r.drafts = append(r.drafts, r.cur)
	r.cur = nil
}

// scopeOrCurrent 返回当前生效的 AOP 作用域。
//
// cur 为 nil 表示尚未开始任何路由，此时作用于控制器级；
// 否则作用于当前路由。
func (r *Routes) scopeOrCurrent(method string) *aopScope {
	if r.cur != nil {
		return &r.cur.aop
	}
	if len(r.drafts) > 0 {
		// 已经 seal 过路由却没有当前路由，说明调用顺序异常（例如在两段声明之间调用）。
		r.fail(method + "() must be followed by a route declaration")
	}
	return &r.scope
}

func (r *Routes) requireCurrent(method string) *routeDraft {
	if r.cur == nil {
		r.fail(method + "() called before any route declaration")
		return nil
	}
	return r.cur
}

func (r *Routes) docModifier(method string, apply func(*RouteDoc)) *Routes {
	cur := r.requireCurrent(method)
	if cur == nil || cur.doc == nil {
		return r
	}
	apply(cur.doc)
	return r
}

func (r *Routes) fail(message string) {
	r.errs = append(r.errs, message)
}

func (r *Routes) build(draft *routeDraft) *RouteDefinition {
	return &RouteDefinition{
		Method:       draft.method,
		Path:         r.prefix + draft.path,
		Controller:   r.controller,
		MethodName:   draft.methodName,
		Guards:       append(append([]aop.Guard{}, r.scope.guards...), draft.aop.guards...),
		Interceptors: append(append([]aop.Interceptor{}, r.scope.interceptors...), draft.aop.interceptors...),
		Middlewares:  append(append([]aop.Middleware{}, r.scope.middlewares...), draft.aop.middlewares...),
		Pipes:        append(append([]aop.Pipe{}, r.scope.pipes...), draft.aop.pipes...),
		ParamPipes:   coreadapter.MergeParamPipes(r.scope.paramPipes, draft.aop.paramPipes),
		Filters:      append(append([]aop.ExceptionFilter{}, r.scope.filters...), draft.aop.filters...),
		StatusCode:   draft.statusCode,
		Headers:      copyHeaders(draft.headers),
		Redirect:     draft.redirect,
		Render:       draft.render,
		Doc:          copyRouteDoc(draft.doc),
	}
}

func copyHeaders(src map[string]string) map[string]string {
	out := make(map[string]string, len(src))
	maps.Copy(out, src)
	return out
}

// copyRouteDoc 做声明期快照，避免多个路由共享底层切片/map。
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
