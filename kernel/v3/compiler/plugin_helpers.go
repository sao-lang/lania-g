// plugin_helpers.go 承载协议 plugin 编译期最通用的扫描、owner 解析和 route 汇总辅助。
//
// 各协议 plugin 的目标是“只保留协议特化逻辑”，
// 因此这里集中收口了可复用的外壳：
// - 从 registry 扫 declaration
// - 基于 snapshot 推导模块 owner
// - 编译单项后汇总为 routes/containers/install
package compiler

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	corescanner "github.com/sao-lang/lania-g/kernel/v3/scanner"
)

// AOPSources 描述某个声明项本地携带的 AOP 定义集合。
//
// 它通常由协议 DSL/声明结构转换而来，随后会在编译期与全局 AOP 合并。
type AOPSources struct {
	Middlewares  []any
	Guards       []any
	Interceptors []any
	Pipes        []any
	ParamPipes   map[int][]any
	Filters      []any
}

// ModuleOwner 表示某个声明项最终归属到的模块信息。
// 编译器最终最关心的就是两件事：`属于哪个模块` 与 `执行时该用哪个容器`。
type ModuleOwner struct {
	ModuleKey  string
	ModuleType reflect.Type
	Container  *di.Container
}

// SnapshotOwnerOptions 控制从 scanner.Snapshot 中提取哪些 owner 信息。
type SnapshotOwnerOptions struct {
	Controllers bool
	Resolvers   bool
	Providers   bool
}

// SnapshotOwnerIndex 是编译器侧用于“模块归属推断”的索引结构。
// 它把 scanner.Snapshot 转成更适合 plugin 读取的结构化索引。
type SnapshotOwnerIndex struct {
	OwnerByType    map[reflect.Type][]ModuleOwner
	OwnerByPointer map[uintptr]ModuleOwner
}

// OwnerResolutionStatus 表示一次 owner 解析的结果状态。
type OwnerResolutionStatus int

const (
	// OwnerResolutionMissing 表示未解析到模块归属。
	OwnerResolutionMissing OwnerResolutionStatus = iota
	// OwnerResolutionResolved 表示成功解析到唯一模块归属。
	OwnerResolutionResolved
	// OwnerResolutionAmbiguous 表示解析到了多个候选归属，结果不唯一。
	OwnerResolutionAmbiguous
)

// OwnerResolution 描述一次模块归属解析的完整结果。
type OwnerResolution struct {
	Token      reflect.Type
	Candidates []ModuleOwner
	Owner      ModuleOwner
	Status     OwnerResolutionStatus
}

// OwnerKindReceiver 表示模块归属来自 receiver 类型。
const OwnerKindReceiver = "receiver"

// CompiledRoute 表示“单个声明项”编译后的标准结果。
// 它会在 CompileRouteSet 中被汇总进 routes / routeContainers。
type CompiledRoute[T any] struct {
	Item      T
	Handler   *runtime.Handler
	RouteKey  string
	Container *di.Container
}

// ScanResolvedRegistryItems 扫描协议声明，解析其模块归属，
// 并投影为协议侧需要的 owner 结构。
func ScanResolvedRegistryItems[T any, O any](
	pluginID string,
	moduleRef *module.ModuleRef,
	reg *registry.Registry,
	nilRegistryMessage string,
	ownerOpts SnapshotOwnerOptions,
	declGroup string,
	cast func(any) (T, bool),
	receiver func(T) any,
	declaration func(T) string,
	meta func(T) map[string]any,
	guidance string,
	build func(T, ModuleOwner, reflect.Type) O,
) ([]O, error) {
	if moduleRef == nil {
		return nil, &kerrors.KernelError{Kind: kerrors.KindDI, Message: "moduleRef is nil", Meta: map[string]interface{}{"stage": "plugin_scan", "plugin": pluginID}}
	}
	if reg == nil {
		return nil, errors.New(nilRegistryMessage)
	}

	snapshot := corescanner.BuildSnapshot(moduleRef)
	owners := BuildSnapshotOwnerIndex(snapshot, ownerOpts)
	items := reg.ListDecl(pluginID, declGroup)
	out := make([]O, 0, len(items))
	for _, raw := range items {
		item, ok := cast(raw)
		if !ok {
			continue
		}
		resolution := owners.Resolve(receiver(item))
		token := resolution.Token
		if token == nil {
			return nil, NewOwnerTargetNilError(pluginID, declaration(item), OwnerKindReceiver, meta(item))
		}
		switch resolution.Status {
		case OwnerResolutionMissing:
			return nil, NewOwnerResolutionError(pluginID, declaration(item), OwnerKindReceiver, token, resolution.Status, resolution.Candidates, meta(item), guidance)
		case OwnerResolutionAmbiguous:
			return nil, NewOwnerResolutionError(pluginID, declaration(item), OwnerKindReceiver, token, resolution.Status, resolution.Candidates, meta(item), "")
		}
		out = append(out, build(item, resolution.Owner, token))
	}
	return out, nil
}

// CompileSimpleProtocol 用于编译“只靠 route map 就能安装”的简单协议。
// 典型如 gRPC/MQ/Scheduler/WS：编译期不需要像 HTTP 那样额外产出一棵 matcher 树。
func CompileSimpleProtocol[T any](
	scan any,
	protocol runtime.Protocol,
	pluginID string,
	invalidScanMessage string,
	items func(any) ([]T, bool),
	global registry.GlobalAOPRegistration,
	compile func(T, registry.GlobalAOPRegistration) (*CompiledRoute[T], error),
	conflictReason func(T) string,
) (*CompiledProtocol, error) {
	owned, ok := items(scan)
	if !ok {
		return nil, &kerrors.KernelError{Kind: kerrors.KindExecution, Message: invalidScanMessage, Meta: map[string]interface{}{"stage": "plugin_compile", "plugin": pluginID}}
	}
	routes, routeContainers, err := CompileRouteSet(owned, func(item T) (*CompiledRoute[T], error) {
		return compile(item, global)
	}, func(item T, routeKey string) error {
		return NewRouteConflictError(RouteConflict{
			RouteKey:       routeKey,
			Protocol:       protocol,
			PluginID:       pluginID,
			ExistingRoute:  routeKey,
			ExistingPlugin: pluginID,
			Reason:         conflictReason(item),
		})
	})
	if err != nil {
		return nil, err
	}
	return &CompiledProtocol{
		Protocol:        protocol,
		Routes:          routes,
		RouteContainers: routeContainers,
		Install:         ProtocolInstaller(protocol, nil, routes),
	}, nil
}

// NewOwnerTargetNilError 构造“owner 目标为空”的统一扫描错误。
func NewOwnerTargetNilError(pluginID, declaration, ownerKind string, meta map[string]any) error {
	return &kerrors.KernelError{
		Kind:    kerrors.KindExecution,
		Message: fmt.Sprintf("module owner target is nil for %s (%s)", declaration, ownerKind),
		Meta:    mergeOwnerErrorMeta(pluginID, ownerKind, "", "nil_target", nil, meta),
	}
}

// NewOwnerResolutionError 构造“owner 缺失/歧义”的统一扫描错误。
func NewOwnerResolutionError(pluginID, declaration, ownerKind string, token reflect.Type, status OwnerResolutionStatus, candidates []ModuleOwner, meta map[string]any, guidance string) error {
	tokenText := ""
	if token != nil {
		tokenText = token.String()
	}
	message := ""
	switch status {
	case OwnerResolutionAmbiguous:
		message = fmt.Sprintf("module owner ambiguous for %s (%s token=%s)", declaration, ownerKind, tokenText)
	default:
		message = fmt.Sprintf("module owner not found for %s (%s token=%s)", declaration, ownerKind, tokenText)
	}
	if extra := strings.TrimSpace(guidance); extra != "" {
		message += "; " + extra
	}
	return &kerrors.KernelError{
		Kind:    kerrors.KindDI,
		Message: message,
		Meta:    mergeOwnerErrorMeta(pluginID, ownerKind, tokenText, ownerStatusText(status), candidates, meta),
	}
}

func ownerStatusText(status OwnerResolutionStatus) string {
	switch status {
	case OwnerResolutionResolved:
		return "resolved"
	case OwnerResolutionAmbiguous:
		return "ambiguous"
	case OwnerResolutionMissing:
		return "missing"
	default:
		return "unknown"
	}
}

func mergeOwnerErrorMeta(pluginID, ownerKind, token, status string, candidates []ModuleOwner, meta map[string]any) map[string]any {
	out := map[string]any{
		"stage":           "plugin_scan",
		"plugin":          pluginID,
		"ownerKind":       ownerKind,
		"ownerStatus":     status,
		"ownerCandidates": ownerCandidateKeys(candidates),
	}
	if strings.TrimSpace(token) != "" {
		out["ownerToken"] = token
	}
	for k, v := range meta {
		out[k] = v
	}
	return out
}

func ownerCandidateKeys(candidates []ModuleOwner) []string {
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		key := strings.TrimSpace(candidate.ModuleKey)
		if key == "" && candidate.ModuleType != nil {
			key = candidate.ModuleType.String()
		}
		if key == "" {
			key = "<unknown>"
		}
		out = append(out, key)
	}
	return out
}

// CompileAOPPlan 将全局 AOP 声明与本地 AOPSources 合并，并转换为 runtime.AOPPlan。
//
// 合并规则：
// - 全局在前，本地在后（本地更靠近 handler）
// - 本地 AOPSources 以 []any 形式接收，支持三种写法：
//   - Func：aop.MiddlewareFunc / GuardFunc / InterceptorFunc / PipeFunc / ExceptionFilterFunc
//   - 对象：aop.Middleware / Guard / Interceptor / Pipe / ExceptionFilter
//   - 构造器：对应的 *Constructor（调用一次创建对象）
func CompileAOPPlan(global registry.GlobalAOPRegistration, local AOPSources) runtime.AOPPlan {
	return runtime.AOPPlan{
		Middlewares:  append(append([]aop.MiddlewareFunc{}, global.Middlewares...), convertMiddlewares(local.Middlewares)...),
		Guards:       append(append([]aop.GuardFunc{}, global.Guards...), convertGuards(local.Guards)...),
		Interceptors: append(append([]aop.InterceptorFunc{}, global.Interceptors...), convertInterceptors(local.Interceptors)...),
		Pipes:        append(append([]aop.PipeFunc{}, global.Pipes...), convertPipes(local.Pipes)...),
		ParamPipes:   convertParamPipes(local.ParamPipes),
		Filters:      append(append([]aop.ExceptionFilterFunc{}, global.Filters...), convertFilters(local.Filters)...),
	}
}

// BuildBoundParamPlans 根据参数类型列表与“参数索引 -> bindingName”的映射生成 ParamPlan 列表。
//
// ParamPlan 会写入 handler.Meta.ParamPlans，供 runtime.BindingRegistry.Resolve 在解析时补全 desc.BindingName。
func BuildBoundParamPlans(paramTypes []reflect.Type, bindings map[int]string) []runtime.ParamPlan {
	out := make([]runtime.ParamPlan, 0, len(paramTypes))
	for i, typ := range paramTypes {
		pp := runtime.ParamPlan{Index: i, Type: typ}
		if name := bindings[i]; name != "" {
			pp.BindingName = name
		}
		out = append(out, pp)
	}
	return out
}

// BuildSnapshotOwnerIndex 将 core/scanner 的 Snapshot 转换为编译器侧的 owner 索引。
//
// index 由两部分组成：
// - OwnerByType：指针类型 token -> owners 列表（可能多 owner）
// - OwnerByPointer：实例指针 -> owner（用于在多 owner 时按实例精确消歧）
//
// 其中：
// - 模块里的 receiver 槽位（controllers / resolvers）会同时贡献类型 owner 与实例指针 owner
// - value provider 若携带实例值，也会贡献实例指针 owner
func BuildSnapshotOwnerIndex(snapshot *corescanner.Snapshot, opts SnapshotOwnerOptions) SnapshotOwnerIndex {
	index := SnapshotOwnerIndex{
		OwnerByType:    make(map[reflect.Type][]ModuleOwner),
		OwnerByPointer: make(map[uintptr]ModuleOwner),
	}
	if snapshot == nil {
		return index
	}
	if opts.Controllers {
		appendSnapshotOwners(index.OwnerByType, snapshot.ControllerOwners)
	}
	if opts.Resolvers {
		appendSnapshotOwners(index.OwnerByType, snapshot.ResolverOwners)
	}
	for _, mod := range snapshot.Modules {
		if mod == nil || mod.Metadata() == nil {
			continue
		}
		owner := ModuleOwner{
			ModuleKey:  reflect.TypeOf(mod).String(),
			ModuleType: reflect.TypeOf(mod),
			Container:  mod.Container(),
		}
		if opts.Controllers {
			for _, controller := range mod.Metadata().Controllers {
				if ptr := InstancePointer(controller); ptr != 0 {
					index.OwnerByPointer[ptr] = owner
				}
			}
		}
		if opts.Resolvers {
			for _, resolver := range mod.Metadata().Resolvers {
				if ptr := InstancePointer(resolver); ptr != 0 {
					index.OwnerByPointer[ptr] = owner
				}
			}
		}
		if opts.Providers {
			for _, provider := range mod.Metadata().Providers {
				if provider == nil {
					continue
				}
				if token, ok := provider.Token.(reflect.Type); ok && token != nil {
					if token.Kind() != reflect.Ptr {
						token = reflect.PointerTo(token)
					}
					index.OwnerByType[token] = append(index.OwnerByType[token], owner)
				}
				if ptr := InstancePointer(provider.UseValue); ptr != 0 {
					index.OwnerByPointer[ptr] = owner
				}
			}
		}
	}
	return index
}

// Resolve 根据实例推导其模块归属（owner）。
//
// 规则：
// - 先根据类型 token（PointerToken）取 candidates
// - candidates=0：未解析到归属（Missing）
// - candidates=1：成功解析到唯一归属（Resolved）
// - candidates>1：尝试用 InstancePointer 精确匹配 OwnerByPointer；匹配到则 Resolved，否则 Ambiguous
func (idx SnapshotOwnerIndex) Resolve(instance any) OwnerResolution {
	token := PointerToken(instance)
	if token == nil {
		return OwnerResolution{Status: OwnerResolutionMissing}
	}
	candidates := idx.OwnerByType[token]
	resolution := OwnerResolution{
		Token:      token,
		Candidates: append([]ModuleOwner{}, candidates...),
		Status:     OwnerResolutionMissing,
	}
	switch len(candidates) {
	case 0:
		return resolution
	case 1:
		resolution.Owner = candidates[0]
		resolution.Status = OwnerResolutionResolved
		return resolution
	default:
		if ptr := InstancePointer(instance); ptr != 0 {
			if owner, ok := idx.OwnerByPointer[ptr]; ok {
				resolution.Owner = owner
				resolution.Status = OwnerResolutionResolved
				return resolution
			}
		}
		resolution.Status = OwnerResolutionAmbiguous
		return resolution
	}
}

// ProtocolInstaller 生成一个安装函数，用于把编译产物（matcher + routes）安装进 runtime.Router。
//
// 该函数通常用于构建 CompiledProtocol.Install。
func ProtocolInstaller(protocol runtime.Protocol, matcher runtime.RouteMatcher, routes map[string]*runtime.Handler) func(*runtime.Router) error {
	return func(router *runtime.Router) error {
		return router.InstallCompiledProtocol(protocol, matcher, routes)
	}
}

// CompileRouteSet 将一组“声明项”编译成 routes 与 routeContainers，并做 routeKey 冲突检测。
// 它是各协议 Compile 阶段最后都会走的一步汇总逻辑。
//
// 参数：
// - items：待编译的声明项列表（例如 HTTP route decl）
// - compile：把单个 item 编译成 CompiledRoute（必须包含 RouteKey/Handler）
// - conflict：冲突回调（可选）；当 routeKey 重复时，用于生成更详细错误
func CompileRouteSet[T any](
	items []T,
	compile func(T) (*CompiledRoute[T], error),
	conflict func(T, string) error,
) (map[string]*runtime.Handler, map[string]*di.Container, error) {
	routes := make(map[string]*runtime.Handler, len(items))
	routeContainers := make(map[string]*di.Container)
	for _, item := range items {
		compiled, err := compile(item)
		if err != nil {
			return nil, nil, err
		}
		if compiled == nil || compiled.Handler == nil || compiled.RouteKey == "" {
			return nil, nil, fmt.Errorf("invalid compiled route")
		}
		if _, exists := routes[compiled.RouteKey]; exists {
			if conflict != nil {
				return nil, nil, conflict(item, compiled.RouteKey)
			}
			return nil, nil, fmt.Errorf("duplicate routeKey: %s", compiled.RouteKey)
		}
		routes[compiled.RouteKey] = compiled.Handler
		if compiled.Container != nil {
			routeContainers[compiled.RouteKey] = compiled.Container
		}
	}
	return routes, routeContainers, nil
}

// PointerToken 将任意值的类型规范化为“指针类型 token”。
//
// 用途：在 container/owner 推导中统一按 *T 做索引，避免 T 与 *T 造成不一致。
func PointerToken(v any) reflect.Type {
	if v == nil {
		return nil
	}
	t := reflect.TypeOf(v)
	if t.Kind() != reflect.Ptr {
		t = reflect.PointerTo(t)
	}
	return t
}

// InstancePointer 返回实例的指针地址（uintptr），用于在“同类型多 owner”场景做消歧。
//
// 注意：
// - 若 v 不是指针且可取地址，会先取 Addr()
// - 若最终不是指针类型，则返回 0
func InstancePointer(v any) uintptr {
	if v == nil {
		return 0
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr && rv.CanAddr() {
		rv = rv.Addr()
	}
	if rv.Kind() != reflect.Ptr {
		return 0
	}
	return rv.Pointer()
}

// convertParamPipes 将参数级 pipes 定义（map[int][]any）转换为 map[int][]aop.PipeFunc。
func convertParamPipes(items map[int][]any) map[int][]aop.PipeFunc {
	if len(items) == 0 {
		return nil
	}
	out := make(map[int][]aop.PipeFunc, len(items))
	for idx, defs := range items {
		out[idx] = convertPipes(defs)
	}
	return out
}

// appendSnapshotOwners 将 scanner 的 owner 结构复制到编译器侧的 owner 结构中，并追加到 dst。
func appendSnapshotOwners(dst map[reflect.Type][]ModuleOwner, src map[reflect.Type][]corescanner.ModuleOwner) {
	for token, owners := range src {
		for _, owner := range owners {
			dst[token] = append(dst[token], ModuleOwner{
				ModuleKey:  owner.ModuleKey,
				ModuleType: owner.ModuleType,
				Container:  owner.Container,
			})
		}
	}
}

// convertMiddlewares 将 middleware 定义列表转换为可执行的 aop.MiddlewareFunc 列表。
func convertMiddlewares(items []any) []aop.MiddlewareFunc {
	out := make([]aop.MiddlewareFunc, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case aop.MiddlewareFunc:
			out = append(out, v)
		case aop.Middleware:
			out = append(out, func(ctx *aop.ExecutionContext, next func() error) error { return v.Use(ctx, next) })
		case aop.MiddlewareConstructor:
			if mw := v(); mw != nil {
				out = append(out, func(ctx *aop.ExecutionContext, next func() error) error { return mw.Use(ctx, next) })
			}
		}
	}
	return out
}

// convertGuards 将 guard 定义列表转换为可执行的 aop.GuardFunc 列表。
func convertGuards(items []any) []aop.GuardFunc {
	out := make([]aop.GuardFunc, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case aop.GuardFunc:
			out = append(out, v)
		case aop.Guard:
			out = append(out, aop.WrapGuard(v))
		case aop.GuardConstructor:
			if g := v(); g != nil {
				out = append(out, aop.WrapGuard(g))
			}
		}
	}
	return out
}

// convertInterceptors 将 interceptor 定义列表转换为可执行的 aop.InterceptorFunc 列表。
func convertInterceptors(items []any) []aop.InterceptorFunc {
	out := make([]aop.InterceptorFunc, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case aop.InterceptorFunc:
			out = append(out, v)
		case aop.Interceptor:
			out = append(out, aop.WrapInterceptor(v))
		case aop.InterceptorConstructor:
			if it := v(); it != nil {
				out = append(out, aop.WrapInterceptor(it))
			}
		}
	}
	return out
}

// convertPipes 将 pipe 定义列表转换为可执行的 aop.PipeFunc 列表。
func convertPipes(items []any) []aop.PipeFunc {
	out := make([]aop.PipeFunc, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case aop.PipeFunc:
			out = append(out, v)
		case aop.Pipe:
			out = append(out, aop.WrapPipe(v))
		case aop.PipeConstructor:
			if p := v(); p != nil {
				out = append(out, aop.WrapPipe(p))
			}
		}
	}
	return out
}

// convertFilters 将 exception filter 定义列表转换为可执行的 aop.ExceptionFilterFunc 列表。
func convertFilters(items []any) []aop.ExceptionFilterFunc {
	out := make([]aop.ExceptionFilterFunc, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case aop.ExceptionFilterFunc:
			out = append(out, v)
		case aop.ExceptionFilter:
			out = append(out, func(exception any, ctx *aop.ExecutionContext) error { return v.Catch(exception, ctx) })
		case aop.ExceptionFilterConstructor:
			if f := v(); f != nil {
				out = append(out, func(exception any, ctx *aop.ExecutionContext) error { return f.Catch(exception, ctx) })
			}
		}
	}
	return out
}
