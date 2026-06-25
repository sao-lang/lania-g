// executor.go 实现 Runtime 在单次请求内的总执行流程。
//
// Executor 负责把这些步骤串起来：
// - 路由匹配
// - request-scope 容器选择
// - binding/DI 参数解析
// - 参数级 pipe
// - 进入 Pipeline 执行完整 AOP 链
package runtime

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
)

// Executor 是 runtime 中每次请求执行的总控组件。
//
// 它负责把“匹配路由、解析参数、选择容器、执行 AOP”这几步串起来。
type Executor struct {
	router          *Router
	pipeline        *Pipeline
	bindingRegistry *BindingRegistry
	rootContainer   *di.Container
	routeContainers map[string]*di.Container
}

const metadataKeyCurrentHandler = "kernel.handler"

// NewExecutor 创建一个执行器（Executor）。
//
// Executor 是每次请求的“总控”：
// - 调 Router 做路由匹配
// - 选择/创建 request-scope 的 DI container
// - 用 BindingRegistry/DI 解析 handler 入参
// - 调 Pipeline 执行 AOP 链并最终调用 handler
func NewExecutor(router *Router, pipeline *Pipeline) *Executor {
	registry := NewBindingRegistry()

	return &Executor{
		router:          router,
		pipeline:        pipeline,
		bindingRegistry: registry,
		routeContainers: make(map[string]*di.Container),
	}
}

// SetRouter 替换 Executor 使用的 Router；传入 nil 会被忽略。
//
// 通常由 Runtime.SetRouter 调用，用于热更新/切换编译产物等场景。
func (e *Executor) SetRouter(router *Router) *Executor {
	if router != nil {
		e.router = router
	}
	return e
}

// WithBindingRegistry 替换参数绑定注册表（BindingRegistry）。
//
// 使用场景：
// - 自定义一套 binding 规则（测试/集成场景）
// - 需要在运行期动态构建 registry
func (e *Executor) WithBindingRegistry(registry *BindingRegistry) *Executor {
	e.bindingRegistry = registry
	return e
}

// SetRootContainer 设置全局（root）DI 容器。
//
// 当某个请求没有命中 routeContainers 的专用容器时，会从 rootContainer.NewChild() 派生 request-scope 容器。
func (e *Executor) SetRootContainer(container *di.Container) *Executor {
	e.rootContainer = container
	return e
}

// SetRouteContainers 设置每个 routeKey 对应的 DI 容器。
//
// 语义：
// - 如果某个 routeKey 存在专用容器，则该路由请求会基于该容器创建 child（container.NewChild()）
// - 否则回退到 rootContainer（如果有）
//
// 典型用途：每个模块/路由拥有独立 provider 注册集合，避免跨模块污染。
func (e *Executor) SetRouteContainers(items map[string]*di.Container) *Executor {
	e.routeContainers = items
	return e
}

// Execute 是 runtime 的统一执行入口：
// - 通过 Router 匹配 handler + 路由参数
// - 为当前请求选择/创建 request-scope 的 DI child container（优先 routeContainers，其次 rootContainer）
// - 通过 BindingRegistry/DI 解析 handler 入参
// - 进入 Pipeline 执行 AOP 链路并调用 handler
func (e *Executor) Execute(ctx *HandlerContext) (interface{}, error) {
	handler, params, err := e.router.Match(ctx)
	if err != nil {
		return nil, e.normalizeError(ctx, err)
	}
	ctx.RouteKey = handler.Meta.RouteKey
	ctx.Set(metadataKeyCurrentHandler, handler)
	if handler.Meta.ModuleKey != "" {
		ctx.Set("kernel.moduleKey", handler.Meta.ModuleKey)
	}

	if ctx.Container == nil {
		// 每次执行都会拿一个 child container，保证 Request scope 的实例不跨请求复用。
		if container, ok := e.routeContainers[handler.Meta.RouteKey]; ok && container != nil {
			ctx.Container = container.NewChild()
		} else if e.rootContainer != nil {
			ctx.Container = e.rootContainer.NewChild()
		}
	}

	// 将 path params 回填到 Request，方便后续 binding resolver 读取。
	for k, v := range params {
		ctx.Request.Params[k] = v
	}

	args, err := e.resolveArguments(ctx, handler)
	if err != nil {
		return nil, err
	}

	result, err := e.pipeline.Run(ctx, handler, args)
	if err != nil {
		return nil, e.normalizeError(ctx, err)
	}
	return result, nil
}

// resolveArguments 解析某个 handler 的全部入参（按声明顺序）。
//
// 每个参数的解析由 resolveArgument 完成：优先 binding，找不到才回退到 DI。
func (e *Executor) resolveArguments(ctx *HandlerContext, handler *Handler) ([]reflect.Value, error) {
	paramTypes := handler.Meta.ParamTypes
	args := make([]reflect.Value, len(paramTypes))

	for i, paramType := range paramTypes {
		value, err := e.resolveArgument(ctx, handler, paramType, i)
		if err != nil {
			return nil, err
		}
		args[i] = value
	}

	return args, nil
}

// resolveArgument 解析单个参数（paramType）。
//
// 规则：
// 1) 先使用 BindingRegistry.Resolve（协议绑定）：例如从 query/body/header/context/path params 提取
// 2) 若 binding 未命中（ErrBindingNotFound），再尝试从 ctx.Container DI 解析
// 3) 解析出的值会做类型校验（AssignableTo/ConvertibleTo），并应用参数级 Pipe（ParamPipes）
func (e *Executor) resolveArgument(ctx *HandlerContext, handler *Handler, paramType reflect.Type, paramIndex int) (reflect.Value, error) {
	// 优先走协议 binding（query/body/header/ctx 等）；如果没有匹配的 binding，再回退到 DI。
	value, err := e.bindingRegistry.Resolve(ctx, handler, paramType, paramIndex)
	if err != nil {
		if err != ErrBindingNotFound {
			return reflect.Value{}, err
		}

		if ctx.Container != nil {
			instance, diErr := ctx.Container.Get(paramType)
			if diErr == nil {
				// binding 未命中时，允许把该参数解释为“从容器注入的依赖”。
				return reflect.ValueOf(instance), nil
			}
			return reflect.Value{}, e.normalizeErrorWithMeta(ctx, fmt.Errorf("%w: %v", ErrDIResolveFailed, diErr), paramIndex, map[string]interface{}{
				"stage": "di_resolve",
			})
		}
		return reflect.Value{}, e.normalizeErrorWithMeta(ctx, ErrBindingNotFound, paramIndex, map[string]interface{}{
			"stage": "binding_resolve",
		})
	}

	current := reflect.ValueOf(value)
	if !current.IsValid() {
		current = reflect.Zero(paramType)
	}
	if current.Type().AssignableTo(paramType) {
		return e.applyParamPipes(ctx, handler, paramIndex, current, paramType)
	}
	if current.Type().ConvertibleTo(paramType) {
		return e.applyParamPipes(ctx, handler, paramIndex, current.Convert(paramType), paramType)
	}
	return reflect.Value{}, e.normalizeErrorWithMeta(ctx, fmt.Errorf("%w: expected %s, got %s", ErrInvalidParamType, paramType.String(), current.Type().String()), paramIndex, map[string]interface{}{
		"stage":    "binding_assign",
		"expected": paramType.String(),
		"actual":   current.Type().String(),
	})
}

// applyParamPipes 执行参数级的 Pipe Transform（仅针对某个参数索引）。
//
// 参数级 Pipe 的来源：
// - 编译期产物：handler.Meta.CompiledAOP.ParamPipes
// - 运行期声明：handler.Meta.ParamPipes
//
// 注意：pipe Transform 的输出仍必须能赋值/转换为目标参数类型，否则会返回 ErrInvalidParamType。
func (e *Executor) applyParamPipes(ctx *HandlerContext, handler *Handler, paramIndex int, value reflect.Value, paramType reflect.Type) (reflect.Value, error) {
	var pipes []aop.PipeFunc
	if handler.Meta.CompiledAOP != nil {
		pipes = handler.Meta.CompiledAOP.ParamPipes[paramIndex]
	} else {
		pipes = handler.Meta.ParamPipes[paramIndex]
	}
	if len(pipes) == 0 {
		return value, nil
	}

	current := value.Interface()
	metadata := &aop.ArgumentMetadata{
		Type:     paramType,
		Data:     fmt.Sprintf("param:%d", paramIndex),
		Metatype: paramType,
	}

	var err error
	for _, pipe := range pipes {
		// 参数级 pipe 在 binding/DI 解析之后执行，
		// 因此看到的是“已经成功拿到的运行时值”，而不是原始字符串/原始输入。
		current, err = pipe.Transform(current, metadata)
		if err != nil {
			return reflect.Value{}, e.normalizeErrorWithMeta(ctx, err, paramIndex, map[string]interface{}{
				"stage": "param_pipe",
			})
		}
	}

	result := reflect.ValueOf(current)
	if !result.IsValid() {
		return reflect.Zero(paramType), nil
	}
	if result.Type().AssignableTo(paramType) {
		return result, nil
	}
	if result.Type().ConvertibleTo(paramType) {
		return result.Convert(paramType), nil
	}
	return reflect.Value{}, e.normalizeErrorWithMeta(ctx, fmt.Errorf("%w: pipe result %s cannot assign to %s", ErrInvalidParamType, result.Type().String(), paramType.String()), paramIndex, map[string]interface{}{
		"stage":  "param_pipe_assign",
		"actual": result.Type().String(),
	})
}

// RegisterBinding 注册一个 BindingResolver。
//
// 这通常由协议 adapter 或 integration 调用，用于新增/覆盖参数解析规则。
func (e *Executor) RegisterBinding(resolver BindingResolver) {
	e.bindingRegistry.Register(resolver)
}

// RegisterBindingFunc 以函数形式注册一个简单的 binding 规则（见 Runtime.RegisterBindingFunc）。
func (e *Executor) RegisterBindingFunc(matcher func(reflect.Type) bool, resolve func(ctx *HandlerContext, paramType reflect.Type) (interface{}, error)) {
	e.bindingRegistry.RegisterFunc(matcher, resolve)
}

// normalizeError 将普通 error 归一化为 KernelError（或保持/包装为框架约定的错误模型）。
//
// 归一化的目的：
// - 统一补全 protocol/routeKey/moduleKey/paramIndex 等诊断信息
// - 让上层 adapter 可以用一致的方式做响应映射与观测上报
func (e *Executor) normalizeError(ctx *HandlerContext, err error) error {
	if err == nil {
		return nil
	}
	return e.normalizeErrorWithMeta(ctx, err, -1, nil)
}

// normalizeErrorWithMeta 在 normalizeError 基础上额外注入 meta 信息。
//
// 典型 meta：
// - stage：错误发生阶段（binding_resolve/di_resolve/param_pipe 等）
// - expected/actual：类型不匹配时的期望/实际类型
func (e *Executor) normalizeErrorWithMeta(ctx *HandlerContext, err error, paramIndex int, meta map[string]interface{}) error {
	if err == nil {
		return nil
	}
	// KernelError 需要尽可能把“在哪个协议/路由/模块/参数阶段”补全，方便排障与观测。
	moduleKey := ""
	if ctx != nil {
		if value, ok := ctx.Get("kernel.moduleKey"); ok {
			moduleKey, _ = value.(string)
		}
	}
	protocol := ""
	routeKey := ""
	if ctx != nil {
		protocol = string(ctx.Protocol)
		routeKey = ctx.RouteKey
	}
	meta = e.enrichMeta(ctx, err, paramIndex, meta)
	return kerrors.NormalizeWithMeta(protocol, routeKey, moduleKey, paramIndex, meta, err)
}

// enrichMeta 尽量从 ctx/routeKey/handler 元信息中提取可诊断字段，合并进 meta。
//
// 例如：
// - method/path：从 routeKey 反解析得到
// - moduleKey：从 ctx.Metadata 读取
// - bindingName/paramType：从 handler.Meta.ParamPlans[paramIndex] 读取
func (e *Executor) enrichMeta(ctx *HandlerContext, err error, paramIndex int, meta map[string]interface{}) map[string]interface{} {
	if meta == nil {
		meta = make(map[string]interface{})
	}
	if paramIndex >= 0 {
		meta["paramIndex"] = paramIndex
	}
	if ctx != nil {
		if ctx.RouteKey != "" {
			if parsed, parseErr := ParseRouteKey(ctx.RouteKey); parseErr == nil {
				if parsed.Method != "" {
					meta["method"] = parsed.Method
				}
				if parsed.Path != "" {
					meta["path"] = parsed.Path
				}
			}
		}
		if moduleKey, ok := ctx.Get("kernel.moduleKey"); ok {
			if value, ok := moduleKey.(string); ok && value != "" {
				meta["moduleKey"] = value
			}
		}
	}
	if paramIndex >= 0 && ctx != nil {
		handler, _ := ctx.Get(metadataKeyCurrentHandler)
		current, _ := handler.(*Handler)
		if current == nil {
			if matched, _, matchErr := e.router.Match(ctx); matchErr == nil {
				current = matched
			}
		}
		if current != nil && paramIndex < len(current.Meta.ParamPlans) {
			if bindingName := current.Meta.ParamPlans[paramIndex].BindingName; bindingName != "" {
				meta["bindingName"] = bindingName
			}
			if paramType := current.Meta.ParamPlans[paramIndex].Type; paramType != nil {
				meta["paramType"] = paramType.String()
			}
		}
	}
	_ = err
	return meta
}
