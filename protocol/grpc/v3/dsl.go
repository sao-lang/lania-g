// dsl.go 提供 gRPC adapter 的声明式注册 DSL。
package grpc

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	coreadapter "github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

// Service 提供一个类似 v2 的全局 DSL 兼容入口：
//
//	b := grpc.Service("UserService", svc).Method("GetUser", svc.GetUser).Build()
//
// 它会把声明写入 `core/registry.Global()`。
// 新业务代码应优先使用挂载在应用实例上的 `adapter.API()`。
func Service(name string, receiver any) *ServiceBuilder {
	return globalCompatAPI("grpc.Service").Service(name, receiver)
}

// API 是 gRPC DSL 对 registry 的轻量封装入口。
type API struct {
	reg            *registry.Registry
	fallbackSource string
}

// NewAPI 创建一个 gRPC DSL 入口。
//
// 推荐：使用挂载到应用实例后的 adapter API，或显式传入实例级 registry。
// 兼容：历史上允许 `NewAPI(nil)`，当前等价于 `NewCompatAPI()`。
func NewAPI(reg *registry.Registry) *API {
	if reg == nil {
		return NewCompatAPI()
	}
	return &API{reg: reg}
}

// NewCompatAPI 创建一个显式保留给迁移场景的全局 DSL 入口，不作为新代码默认入口。
func NewCompatAPI() *API {
	return globalCompatAPI("grpc.NewCompatAPI()")
}

func globalCompatAPI(source string) *API {
	return &API{reg: registry.Global(), fallbackSource: source}
}

// Service 创建一个 gRPC service 声明构建器。
func (api *API) Service(name string, receiver any) *ServiceBuilder {
	return newServiceBuilder(name, receiver, api.reg, api.fallbackSource)
}

// ServiceBuilder 用于声明一个 gRPC service 及其共享配置。
type ServiceBuilder struct {
	service        string
	receiver       any
	methods        []*MethodBuilder
	guards         []any
	interceptors   []any
	middlewares    []any
	pipes          []any
	paramPipes     map[int][]any
	filters        []any
	registry       *registry.Registry
	fallbackSource string
	sealed         bool
	mu             sync.RWMutex
	err            error
}

// MethodBuilder 用于声明一个具体的 RPC 方法。
type MethodBuilder struct {
	serviceBuilder *ServiceBuilder
	method         string
	// mode 让 DSL 在声明期就把四种调用模式显式固定下来，
	// 避免后续再通过函数签名“反推模式”导致诊断和行为不稳定。
	mode           RPCMode
	handler        any
	handlerName    string

	paramBindings map[int]string
	// reqType/respType 都是“可选显式覆盖”：
	// 默认仍尽量从业务签名推断，只有在推断会歧义时才需要业务手动指定。
	reqType       reflect.Type
	respType      reflect.Type

	guards       []any
	interceptors []any
	middlewares  []any
	pipes        []any
	paramPipes   map[int][]any
	filters      []any
}

func newServiceBuilder(name string, receiver any, reg *registry.Registry, fallbackSource string) *ServiceBuilder {
	if reg == nil {
		reg = registry.Global()
	}
	return &ServiceBuilder{
		service:        name,
		receiver:       receiver,
		methods:        make([]*MethodBuilder, 0),
		paramPipes:     make(map[int][]any),
		registry:       reg,
		fallbackSource: fallbackSource,
	}
}

// UseGuards 在 service 级别追加守卫，后续方法都会继承。
func (b *ServiceBuilder) UseGuards(items ...any) *ServiceBuilder {
	b.assertCanConfigureServiceScope()
	b.guards = append(b.guards, items...)
	return b
}

// UseInterceptors 在 service 级别追加拦截器，后续方法都会继承。
func (b *ServiceBuilder) UseInterceptors(items ...any) *ServiceBuilder {
	b.assertCanConfigureServiceScope()
	b.interceptors = append(b.interceptors, items...)
	return b
}

// UseMiddlewares 在 service 级别追加中间件，后续方法都会继承。
func (b *ServiceBuilder) UseMiddlewares(items ...any) *ServiceBuilder {
	b.assertCanConfigureServiceScope()
	b.middlewares = append(b.middlewares, items...)
	return b
}

// UsePipes 在 service 级别追加 Pipe，后续方法都会继承。
func (b *ServiceBuilder) UsePipes(items ...any) *ServiceBuilder {
	b.assertCanConfigureServiceScope()
	b.pipes = append(b.pipes, items...)
	return b
}

// UseParamPipes 在 service 级别追加参数级 Pipe，后续方法都会继承。
func (b *ServiceBuilder) UseParamPipes(paramIndex int, pipes ...any) *ServiceBuilder {
	b.assertCanConfigureServiceScope()
	if b.paramPipes[paramIndex] == nil {
		b.paramPipes[paramIndex] = make([]any, 0)
	}
	b.paramPipes[paramIndex] = append(b.paramPipes[paramIndex], pipes...)
	return b
}

// UseFilters 在 service 级别追加异常过滤器，后续方法都会继承。
func (b *ServiceBuilder) UseFilters(items ...any) *ServiceBuilder {
	b.assertCanConfigureServiceScope()
	b.filters = append(b.filters, items...)
	return b
}

// Method 用于声明一个 unary RPC 方法。
// `method` 是 proto 中的 RPC 名称（例如 `GetUser`），`handler` 通常是 `receiver.GetUser`。
func (b *ServiceBuilder) Method(method string, handler any) *MethodBuilder {
	return b.newMethod(method, RPCModeUnary, handler)
}

// ServerStreamMethod 用于声明一个服务端流式 RPC 方法。
func (b *ServiceBuilder) ServerStreamMethod(method string, handler any) *MethodBuilder {
	return b.newMethod(method, RPCModeServerStream, handler)
}

// ClientStreamMethod 用于声明一个客户端流式 RPC 方法。
func (b *ServiceBuilder) ClientStreamMethod(method string, handler any) *MethodBuilder {
	return b.newMethod(method, RPCModeClientStream, handler)
}

// BidiStreamMethod 用于声明一个双向流式 RPC 方法。
func (b *ServiceBuilder) BidiStreamMethod(method string, handler any) *MethodBuilder {
	return b.newMethod(method, RPCModeBidiStream, handler)
}

// newMethod 是四种 DSL 声明入口的统一落点。
// 所有 AOP 配置、绑定名、显式消息类型覆盖最终都会落在同一个 MethodBuilder 结构上，
// 这样可以保证 unary 和 streaming 的声明体验尽量一致，只在 mode 上分叉。
func (b *ServiceBuilder) newMethod(method string, mode RPCMode, handler any) *MethodBuilder {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sealed {
		return nil
	}

	mb := &MethodBuilder{
		serviceBuilder: b,
		method:         method,
		mode:           mode,
		handler:        handler,
		handlerName:    coreadapter.FindMethodName(b.receiver, handler),
		paramBindings:  make(map[int]string),
		guards:         make([]any, 0),
		interceptors:   make([]any, 0),
		middlewares:    make([]any, 0),
		pipes:          make([]any, 0),
		paramPipes:     make(map[int][]any),
		filters:        make([]any, 0),
	}
	// 一个 service 可以同时声明多种模式的方法；
	// 构建顺序会被保留，最终在 BuildE 时统一转成 MethodDefinition 列表。
	b.methods = append(b.methods, mb)
	return mb
}

// UseGuards 为当前 RPC 方法追加 guards。
func (mb *MethodBuilder) UseGuards(items ...any) *MethodBuilder {
	mb.guards = append(mb.guards, items...)
	return mb
}

// UseInterceptors 为当前 RPC 方法追加 interceptors。
func (mb *MethodBuilder) UseInterceptors(items ...any) *MethodBuilder {
	mb.interceptors = append(mb.interceptors, items...)
	return mb
}

// UseMiddlewares 为当前 RPC 方法追加 middlewares。
func (mb *MethodBuilder) UseMiddlewares(items ...any) *MethodBuilder {
	mb.middlewares = append(mb.middlewares, items...)
	return mb
}

// UsePipes 为当前 RPC 方法追加 pipes。
func (mb *MethodBuilder) UsePipes(items ...any) *MethodBuilder {
	mb.pipes = append(mb.pipes, items...)
	return mb
}

// UseParamPipes 为指定参数位置追加 pipes。
func (mb *MethodBuilder) UseParamPipes(paramIndex int, pipes ...any) *MethodBuilder {
	if mb.paramPipes[paramIndex] == nil {
		mb.paramPipes[paramIndex] = make([]any, 0)
	}
	mb.paramPipes[paramIndex] = append(mb.paramPipes[paramIndex], pipes...)
	return mb
}

// UseFilters 为当前 RPC 方法追加 filters。
func (mb *MethodBuilder) UseFilters(items ...any) *MethodBuilder {
	mb.filters = append(mb.filters, items...)
	return mb
}

// BindParam 为指定参数位置设置 binding 名称。
// 例如对 `grpcbinding.Header[T]`，这里的 `name` 就是 metadata key。
func (mb *MethodBuilder) BindParam(paramIndex int, name string) *MethodBuilder {
	if name == "" {
		return mb
	}
	// gRPC metadata key 会统一规范化为小写。
	mb.paramBindings[paramIndex] = strings.ToLower(name)
	return mb
}

// WithReqType 显式指定该方法的请求消息类型（通常是 `*pb.FooRequest`）。
// 当你不希望依赖 transport 推断时，这个方法会很有用。
// 既接受 `reflect.Type`，也接受一个示例值（推荐传指针类型）。
func (mb *MethodBuilder) WithReqType(t any) *MethodBuilder {
	if t == nil {
		mb.reqType = nil
		return mb
	}
	switch v := t.(type) {
	case reflect.Type:
		mb.reqType = v
	default:
		mb.reqType = reflect.TypeOf(t)
	}
	// gRPC 解码器和我们后续的 transport 逻辑都更偏向拿到“消息指针类型”，
	// 因此这里统一把非指针输入提升成指针，减少后续分支复杂度。
	if mb.reqType != nil && mb.reqType.Kind() != reflect.Ptr {
		mb.reqType = reflect.PointerTo(mb.reqType)
	}
	return mb
}

// WithRespType 显式指定该方法的响应消息类型。
func (mb *MethodBuilder) WithRespType(t any) *MethodBuilder {
	if t == nil {
		mb.respType = nil
		return mb
	}
	switch v := t.(type) {
	case reflect.Type:
		mb.respType = v
	default:
		mb.respType = reflect.TypeOf(t)
	}
	// 和请求类型一样，响应类型也统一规整成指针形式，便于后续扩展更严格的类型诊断。
	if mb.respType != nil && mb.respType.Kind() != reflect.Ptr {
		mb.respType = reflect.PointerTo(mb.respType)
	}
	return mb
}

// Method 在当前 service 下继续声明下一条 RPC 方法。
func (mb *MethodBuilder) Method(method string, handler any) *MethodBuilder {
	return mb.serviceBuilder.Method(method, handler)
}

// ServerStreamMethod 在当前 service 下继续声明下一条服务端流式 RPC 方法。
func (mb *MethodBuilder) ServerStreamMethod(method string, handler any) *MethodBuilder {
	return mb.serviceBuilder.ServerStreamMethod(method, handler)
}

// ClientStreamMethod 在当前 service 下继续声明下一条客户端流式 RPC 方法。
func (mb *MethodBuilder) ClientStreamMethod(method string, handler any) *MethodBuilder {
	return mb.serviceBuilder.ClientStreamMethod(method, handler)
}

// BidiStreamMethod 在当前 service 下继续声明下一条双向流式 RPC 方法。
func (mb *MethodBuilder) BidiStreamMethod(method string, handler any) *MethodBuilder {
	return mb.serviceBuilder.BidiStreamMethod(method, handler)
}

// Build 返回当前 service 已声明的方法定义。
func (mb *MethodBuilder) Build() []*MethodDefinition { return mb.serviceBuilder.Build() }

// Build 返回当前 service 收集到的方法定义；忽略构建错误。
func (b *ServiceBuilder) Build() []*MethodDefinition {
	defs, _ := b.BuildE()
	return defs
}

// BuildE 完成当前 service 的声明收集，并把方法注册进 registry。
func (b *ServiceBuilder) BuildE() ([]*MethodDefinition, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return nil, b.err
	}
	b.sealed = true

	defs := make([]*MethodDefinition, 0, len(b.methods))
	for _, mb := range b.methods {
		defs = append(defs, mb.build(b))
	}

	items := make([]any, 0, len(defs))
	for _, def := range defs {
		items = append(items, def)
	}
	if b.fallbackSource != "" {
		b.registry.RecordFallbackUsage(b.fallbackSource)
	}
	// registry 中仍然只保存统一的 MethodDefinition 列表；
	// 后续到底注册成 `MethodDesc` 还是 `StreamDesc`，由 adapter 在启动时再按 mode 分发。
	b.registry.RegisterDecl(AdapterID, "methods", items...)
	return defs, nil
}

// Register 构建并返回当前 service 的方法定义。
func (b *ServiceBuilder) Register() []*MethodDefinition { return b.Build() }

// Err 返回 service 级 DSL 声明过程中记录的错误。
func (b *ServiceBuilder) Err() error { return b.err }

func (mb *MethodBuilder) build(sb *ServiceBuilder) *MethodDefinition {
	return &MethodDefinition{
		Service:       sb.service,
		Method:        mb.method,
		Mode:          mb.mode,
		Receiver:      sb.receiver,
		HandlerName:   mb.handlerName,
		RequestType:   mb.reqType,
		ResponseType:  mb.respType,
		// 这里全部做拷贝，避免 builder 在 Build 后被误复用时污染已经产出的声明对象。
		ParamBindings: coreadapter.CopyIntStringMap(mb.paramBindings),
		Guards:        append(append([]any{}, sb.guards...), mb.guards...),
		Interceptors:  append(append([]any{}, sb.interceptors...), mb.interceptors...),
		Middlewares:   append(append([]any{}, sb.middlewares...), mb.middlewares...),
		Pipes:         append(append([]any{}, sb.pipes...), mb.pipes...),
		ParamPipes:    coreadapter.MergeParamPipes(sb.paramPipes, mb.paramPipes),
		Filters:       append(append([]any{}, sb.filters...), mb.filters...),
	}
}

func (b *ServiceBuilder) assertCanConfigureServiceScope() {
	if len(b.methods) > 0 && b.err == nil {
		b.err = fmt.Errorf("grpc.ServiceBuilder: service-level Use* APIs must be called before declaring the first method")
	}
}
