---
title: Registry / Compiler / Runtime
weight: 3
prev: /docs/reference/module-di
next: /docs/reference/aop
---

三个内核包共同完成「声明 → 编译 → 执行」。

## 1. Registry

包：`kernel/v3/registry`

声明只写进 Registry，不直接产生可执行对象。

```go
func New() *Registry
func Global() *Registry
func ResetGlobal()
func (r *Registry) RegisterDecl(protocol, category string, items ...any)
func (r *Registry) ListDecl(protocol, category string) []any
func (r *Registry) RegisterBindings(bindings ...runtime.BindingResolver)
func (r *Registry) Bindings() []runtime.BindingResolver
func (r *Registry) RegisterGlobalAOP(reg GlobalAOPRegistration)
func (r *Registry) GlobalAOP() GlobalAOPRegistration
func (r *Registry) RecordFallbackUsage(source string)
func (r *Registry) SnapshotFallbackUsage() map[string]int
func (r *Registry) Reset()
```

### 声明分类（按协议）

| protocol | category | 内容 |
|----------|----------|------|
| `http` | `routes` | HTTP 路由声明 |
| `grpc` | `services` / `methods` | gRPC 服务与方法 |
| `graphql` | `resolvers` / `config` | Resolver 与 schema 配置 |
| `ws` | `handlers` / `gateways` | 网关与事件 |
| `mq` | `consumers` / `subscriptions` | 消费者与订阅 |
| `scheduler` | `jobs` | 任务声明 |
| 任意 | `bindings` | Binding resolver |

### 全局 AOP 注册

```go
type GlobalAOPRegistration struct {
	Middlewares  []aop.MiddlewareFunc
	Guards       []aop.GuardFunc
	Interceptors []aop.InterceptorFunc
	Pipes        []aop.PipeFunc
	Filters      []aop.ExceptionFilterFunc
}
```

## 2. Compiler

包：`kernel/v3/compiler`

```go
func Compile(moduleRef *module.ModuleRef, reg *registry.Registry,
	plugins ...ProtocolPlugin) (*CompiledApp, error)

func CompileCompat(moduleRef *module.ModuleRef, plugins ...ProtocolPlugin) (*CompiledApp, error)
```

### ProtocolPlugin

```go
type ProtocolPlugin interface {
	ID() string
	Protocol() runtime.Protocol
	Register(reg *registry.Registry)
	Scan(moduleRef *module.ModuleRef, reg *registry.Registry) (any, error)
	Compile(scan any, reg *registry.Registry,
		global registry.GlobalAOPRegistration) (*CompiledProtocol, error)
}
```

### CompiledApp

```go
func (c *CompiledApp) Install(rt *runtime.Runtime) error
```

安装内容：

- Router：routeKey → Handler
- BindingRegistry：各协议 resolver
- RouteContainers：路由 → 模块容器

### 编译诊断

```go
type CompileDiagnostics struct {
	RegisteredPlugins        []string
	ProtocolOrder            []runtime.Protocol
	DeclarationCounts        map[string]map[string]int
	BindingResolverCount     int
	RegistrySource           string
	GlobalAOP                AOPDiagnostics
	Protocols                map[runtime.Protocol]*ProtocolDiagnostics
	RegistryFallbacks        []RegistryFallbackDiagnostics
	RuntimeFallbacks         []RuntimeFallbackDiagnostics
	CompatFallbackCategories []CompatFallbackCategorySummary
	CompatFallbackSources    []CompatFallbackSourceSummary
	RouteConflicts           []RouteConflict
	Errors                   []string
	Warnings                 []string
}
```

## 3. Runtime

包：`kernel/v3/runtime`

### Handler

```go
func NewHandler(instance any, methodName string) (*Handler, error)
func NewHandlerByToken(token any, methodName string) (*Handler, error)
func (h *Handler) Invoke(ctx *HandlerContext, args []reflect.Value) ([]reflect.Value, error)
func (h *Handler) NumParams() int
func (h *Handler) NumReturns() int
```

`HandlerMeta` 关键字段：

| 字段 | 含义 |
|------|------|
| `RouteKey` | 路由标识 |
| `Protocol` | 所属协议 |
| `ParamTypes` / `ParamPlans` | 参数类型与绑定计划 |
| `CompiledAOP` | 编译期 AOP 计划 |
| `StatusCode` / `Headers` / `RedirectURL` | 响应元信息 |

### HandlerContext

```go
type HandlerContext struct {
	Protocol  Protocol
	RouteKey  string
	Container *di.Container
	Request   *Request
	Response  *Response
	Metadata  map[string]interface{}
}

func AcquireHandlerContext(protocol Protocol) *HandlerContext
func NewHandlerContext(protocol Protocol) *HandlerContext
func ReleaseHandlerContext(hc *HandlerContext)
```

| 方法 | 说明 |
|------|------|
| `Set/Get/MustGet/Require` | metadata 读写 |
| `WithContext` / `WithTimeout` | 绑定 `context.Context` |
| `Context/Deadline/Done/Err/Value` | 实现 `context.Context` |

### Request / Response

```go
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

type Response struct {
	Status  int
	Headers map[string]string
	Body    interface{}
	Raw     interface{}
}
```

### Executor

```go
func NewExecutor(router *Router, pipeline *Pipeline) *Executor
func (e *Executor) Execute(ctx *HandlerContext) (any, error)
func (e *Executor) WithBindingRegistry(reg *BindingRegistry) *Executor
func (e *Executor) SetRootContainer(c *di.Container) *Executor
func (e *Executor) SetRouteContainers(m map[string]*di.Container) *Executor
func (e *Executor) SetRouter(r *Router) *Executor
func (e *Executor) RegisterBinding(resolver BindingResolver)
func (e *Executor) RegisterBindingFunc(matcher func(reflect.Type) bool,
	resolve func(*HandlerContext, reflect.Type) (any, error))
```

### BindingRegistry

```go
func NewBindingRegistry() *BindingRegistry
func (br *BindingRegistry) Register(resolver BindingResolver)
func (br *BindingRegistry) RegisterBinding(reg BindingRegistration)
func (br *BindingRegistry) RegisterFunc(matcher func(reflect.Type) bool, resolve ...)
func (br *BindingRegistry) Find(t reflect.Type) (BindingResolver, WrapperDescriptor, bool)
func (br *BindingRegistry) Resolve(ctx *HandlerContext, h *Handler,
	t reflect.Type, index int) (any, error)
```

```go
type BindingRegistration struct {
	Name             string
	AllowedProtocols map[Protocol]bool
	Match            func(reflect.Type) (WrapperDescriptor, bool)
	Resolve          func(*HandlerContext, WrapperDescriptor) (any, error)
}
```

### Pipeline

```go
func (p *Pipeline) Run(ctx *HandlerContext, h *Handler, args []reflect.Value) (any, error)
func (p *Pipeline) UseGlobalMiddlewares(...)
func (p *Pipeline) UseGlobalGuards(...)
func (p *Pipeline) UseGlobalInterceptors(...)
func (p *Pipeline) UseGlobalPipes(...)
func (p *Pipeline) UseGlobalFilters(...)
```

### 路由

```go
func BuildRouteKey(protocol Protocol, method, path string) string
func ParseRouteKey(key string) (RouteKey, error)
```

格式：`<protocol>:<method>:<path>`，例如 `http:GET:/users/:id`、
`ws:users_list:/ws/chat`、`scheduler:every-job:<receiver>`。
