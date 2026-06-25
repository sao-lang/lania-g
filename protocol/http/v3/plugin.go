// plugin.go 实现 HTTP 协议的编译插件。
package http

import (
	"fmt"
	"maps"
	"reflect"
	"strings"

	httpbinding "github.com/sao-lang/lania-g/protocol/http/v3/binding"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	httpprotocol "github.com/sao-lang/lania-g/protocol/http/v3/protocol"
)

// Plugin 是 HTTP 协议的编译插件。
type Plugin struct{}

// NewPlugin 创建一个 HTTP 协议插件。
func NewPlugin() compiler.ProtocolPlugin { return &Plugin{} }

// ID 返回插件标识。
func (p *Plugin) ID() string { return AdapterID }

// Protocol 返回插件对应的协议类型。
func (p *Plugin) Protocol() runtime.Protocol { return httpprotocol.Protocol }

// Register 向 registry 注册 HTTP 默认 binding。
func (p *Plugin) Register(reg *registry.Registry) {
	httpbinding.RegisterDefaultsToRegistry(reg)
}

type routeOwnership struct {
	definition      *RouteDefinition
	moduleKey       string
	moduleType      reflect.Type
	moduleContainer *di.Container
	controllerToken reflect.Type
}

type scanResult struct {
	routes []*routeOwnership
}

// Scan 扫描 registry 中声明的 HTTP 路由，并解析其所属模块与容器。
//
// 这一层只做“声明归属”问题：
// - 从 registry 取出 routes declaration
// - 结合 module snapshot 判断 controller 属于哪个模块 owner
// - 把 declaration 扩展成带 container/token 的 routeOwnership
//
// 真正的 handler 编译、AOP 合并、路由树安装都留在 Compile 阶段完成。
func (p *Plugin) Scan(moduleRef *module.ModuleRef, reg *registry.Registry) (any, error) {
	routes, err := compiler.ScanResolvedRegistryItems(AdapterID, moduleRef, reg,
		"http plugin scan requires an explicit registry; pass the application registry instance or registry.Global() explicitly for compatibility",
		compiler.SnapshotOwnerOptions{Controllers: true},
		"routes",
		func(raw any) (*RouteDefinition, bool) {
			def, ok := raw.(*RouteDefinition)
			return def, ok && def != nil
		},
		func(def *RouteDefinition) any { return def.Controller },
		func(def *RouteDefinition) string { return fmt.Sprintf("http route %s %s", def.Method, def.Path) },
		func(def *RouteDefinition) map[string]any {
			return map[string]any{
				"path":   def.Path,
				"method": def.Method,
			}
		},
		"",
		func(def *RouteDefinition, own compiler.ModuleOwner, token reflect.Type) *routeOwnership {
			return &routeOwnership{
				definition:      def,
				moduleKey:       own.ModuleKey,
				moduleType:      own.ModuleType,
				moduleContainer: own.Container,
				controllerToken: token,
			}
		},
	)
	if err != nil {
		return nil, err
	}
	return &scanResult{routes: routes}, nil
}

// Compile 把扫描结果编译为运行期可安装的 HTTP 协议产物。
//
// HTTP 与 ws/grpc/mq 的差别在于：这里除了 routeKey -> handler 之外，
// 还要额外构建一棵按 method/path 组织的树，再在 Install 阶段回放到 matcher。
func (p *Plugin) Compile(scan any, reg *registry.Registry, global registry.GlobalAOPRegistration) (*compiler.CompiledProtocol, error) {
	result, ok := scan.(*scanResult)
	if !ok || result == nil {
		return nil, &kerrors.KernelError{Kind: kerrors.KindExecution, Message: "invalid scan result for http plugin", Meta: map[string]any{"stage": "plugin_compile", "plugin": AdapterID}}
	}

	tree := &runtime.CompiledHTTPTree{
		Methods: make(map[string]*runtime.CompiledHTTPNode),
		All:     &runtime.CompiledHTTPNode{Static: make(map[string]*runtime.CompiledHTTPNode)},
	}
	routes, routeContainers, err := compiler.CompileRouteSet(result.routes, func(owned *routeOwnership) (*compiler.CompiledRoute[*routeOwnership], error) {
		handler, routeKey, err := compileRoute(owned, global)
		if err != nil {
			return nil, err
		}
		// 先记录编译期树，再把 routeKey 对应的 handler 存进 routes map。
		// Install 阶段会把这棵树重新回放到具体 matcher 实现里。
		insertCompiledRoute(tree, string(owned.definition.Method), owned.definition.Path, routeKey)
		return &compiler.CompiledRoute[*routeOwnership]{
			Item:      owned,
			Handler:   handler,
			RouteKey:  routeKey,
			Container: owned.moduleContainer,
		}, nil
	}, func(owned *routeOwnership, routeKey string) error {
		return compiler.NewRouteConflictError(compiler.RouteConflict{
			RouteKey:       routeKey,
			Protocol:       httpprotocol.Protocol,
			PluginID:       AdapterID,
			ExistingRoute:  routeKey,
			ExistingPlugin: AdapterID,
			Reason:         fmt.Sprintf("duplicate http route %s %s", owned.definition.Method, owned.definition.Path),
		})
	})
	if err != nil {
		return nil, err
	}

	return &compiler.CompiledProtocol{
		Protocol:        httpprotocol.Protocol,
		Routes:          routes,
		RouteContainers: routeContainers,
		Install: func(router *runtime.Router) error {
			matcher := newRadixMatcher()
			for method, root := range tree.Methods {
				replayCompiledTree(matcher, method, "", root, routes)
			}
			replayCompiledTree(matcher, httpprotocol.AllMethod, "", tree.All, routes)
			return compiler.ProtocolInstaller(httpprotocol.Protocol, matcher, routes)(router)
		},
	}, nil
}

// compileRoute 把单条 HTTP route declaration 编译成 runtime.Handler。
// 这里保留 HTTP 特有逻辑，例如：
// - 从 path 推导 `Param[...]` 的 BindingName
// - 写入状态码、重定向、模板渲染、固定响应头等 HTTP 元数据
func compileRoute(route *routeOwnership, global registry.GlobalAOPRegistration) (*runtime.Handler, string, error) {
	def := route.definition
	if def == nil {
		return nil, "", fmt.Errorf("nil http route definition")
	}
	if def.Controller == nil || def.MethodName == "" {
		return nil, "", fmt.Errorf("invalid http route declaration: %s %s", def.Method, def.Path)
	}

	handler, err := runtime.NewHandlerByToken(route.controllerToken, def.MethodName)
	if err != nil {
		return nil, "", err
	}

	plan := compiler.CompileAOPPlan(global, compiler.AOPSources{
		Middlewares:  def.Middlewares,
		Guards:       def.Guards,
		Interceptors: def.Interceptors,
		Pipes:        def.Pipes,
		ParamPipes:   def.ParamPipes,
		Filters:      def.Filters,
	})

	paramPlan := buildParamPlan(handler, def.Path)
	handler.Meta.ParamPlans = paramPlan
	handler.Meta.CompiledAOP = &plan
	handler.Meta.StatusCode = def.StatusCode
	if def.Redirect != nil {
		handler.Meta.RedirectURL = def.Redirect.URL
		handler.Meta.RedirectCode = def.Redirect.Status
	}
	handler.Meta.Render = def.Render
	maps.Copy(handler.Meta.Headers, def.Headers)

	// routeKey 是运行时查找 handler 的唯一键，格式统一交给 runtime.BuildRouteKey 生成。
	routeKey := runtime.BuildRouteKey(httpprotocol.Protocol, string(def.Method), def.Path)
	handler.Meta.RouteKey = routeKey
	handler.Meta.Protocol = httpprotocol.Protocol
	handler.Meta.ModuleKey = route.moduleKey

	return handler, routeKey, nil
}

// buildParamPlan 负责把 handler 的形参列表补成运行时参数计划。
// HTTP 这里的关键附加信息是路径参数名，例如 `/users/:id` -> `Param[string]{BindingName:"id"}`。
func buildParamPlan(handler *runtime.Handler, path string) []runtime.ParamPlan {
	out := make([]runtime.ParamPlan, 0, len(handler.Meta.ParamTypes))
	paramNames := extractHTTPParamNames(path)
	paramNameIndex := 0
	for i, typ := range handler.Meta.ParamTypes {
		plan := runtime.ParamPlan{Index: i, Type: typ}
		if isParamWrapper(typ) && paramNameIndex < len(paramNames) {
			// 只给按出现顺序匹配到的 `Param[...]` 形参写名字，
			// 这样运行时无需再解析路由模板。
			plan.BindingName = paramNames[paramNameIndex]
			paramNameIndex++
		}
		out = append(out, plan)
	}
	return out
}

func isParamWrapper(t reflect.Type) bool {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Kind() == reflect.Struct && t.PkgPath() == httpbinding.PackagePath() && trimGenericName(t.Name()) == "Param"
}

func trimGenericName(name string) string {
	if prefix, _, ok := strings.Cut(name, "["); ok {
		return prefix
	}
	return name
}

func extractHTTPParamNames(path string) []string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	names := make([]string, 0)
	for _, part := range parts {
		if strings.HasPrefix(part, ":") && len(part) > 1 {
			names = append(names, part[1:])
		}
	}
	return names
}

// insertCompiledRoute 把编译结果写入一棵轻量 HTTP 路由树。
// 这棵树是“安装前的中间结构”，便于后续统一回放到 radix matcher。
func insertCompiledRoute(tree *runtime.CompiledHTTPTree, method, path, routeKey string) {
	if tree == nil {
		return
	}
	var root *runtime.CompiledHTTPNode
	if method == httpprotocol.AllMethod {
		root = tree.All
	} else {
		root = tree.Methods[method]
		if root == nil {
			root = &runtime.CompiledHTTPNode{Static: make(map[string]*runtime.CompiledHTTPNode)}
			tree.Methods[method] = root
		}
	}
	segs := strings.Split(strings.Trim(path, "/"), "/")
	node := root
	for _, seg := range segs {
		if seg == "" {
			continue
		}
		if strings.HasPrefix(seg, ":") && len(seg) > 1 {
			// 动态段统一落在 Param 分支上，静态段优先走 Static 分支。
			if node.Param == nil {
				node.Param = &runtime.CompiledHTTPNode{Static: make(map[string]*runtime.CompiledHTTPNode), ParamName: seg[1:]}
			}
			node = node.Param
			continue
		}
		if node.Static == nil {
			node.Static = make(map[string]*runtime.CompiledHTTPNode)
		}
		next := node.Static[seg]
		if next == nil {
			next = &runtime.CompiledHTTPNode{Static: make(map[string]*runtime.CompiledHTTPNode)}
			node.Static[seg] = next
		}
		node = next
	}
	node.RouteKey = routeKey
}

// replayCompiledTree 把编译期树重新铺回 matcher。
// Compile 阶段只关心“有什么路由”；真正依赖具体 matcher API 的安装动作延后到这里。
func replayCompiledTree(matcher routeMatcher, method, prefix string, node *runtime.CompiledHTTPNode, routes map[string]*runtime.Handler) {
	if node == nil {
		return
	}
	if node.RouteKey != "" {
		if handler := routes[node.RouteKey]; handler != nil {
			path := prefix
			if path == "" {
				path = "/"
			}
			matcher.Add(method, path, handler)
		}
	}
	for seg, child := range node.Static {
		nextPrefix := prefix
		if nextPrefix == "" {
			nextPrefix = "/" + seg
		} else {
			nextPrefix += "/" + seg
		}
		replayCompiledTree(matcher, method, nextPrefix, child, routes)
	}
	if node.Param != nil {
		nextPrefix := prefix
		if nextPrefix == "" {
			nextPrefix = "/:" + node.Param.ParamName
		} else {
			nextPrefix += "/:" + node.Param.ParamName
		}
		replayCompiledTree(matcher, method, nextPrefix, node.Param, routes)
	}
}
