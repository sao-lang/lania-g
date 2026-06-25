// registry.go 实现框架编译期声明仓库。
//
// 这一层是“单一事实来源”：
// - adapter/DSL 往里写 declaration
// - application 往里写全局 AOP
// - binding 注册往里写 resolver
// - compiler 最终从这里读取并生成运行时产物
package registry

import (
	"maps"
	"sync"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// Registry 是编译期声明的“单一事实来源”。
// 它主要存放：
// - 各协议写入的声明（例如 HTTP 路由、GraphQL resolver、Scheduler job）
// - 全局 AOP 声明
// - 编译阶段会被组装进 `runtime.BindingRegistry` 的 binding resolvers
//
// 之所以放在 core，是因为 compiler/scanner/runtime 都依赖它。
type Registry struct {
	mu sync.RWMutex

	// decls is a generic declaration store to support pluggable protocol plugins.
	// Keyed by pluginID -> kind -> []any.
	// 约定：compiler 只关心“按 plugin/kind 汇总后的声明列表”，具体 item 的结构由 plugin 自己定义。
	decls map[string]map[string][]any

	// globalAOP 存放全局 AOP（由 application 侧的 sugar 方法写入），compile 时会被展开到每个 handler。
	globalAOP GlobalAOPRegistration

	// bindings 是 compile 时组装 runtime.BindingRegistry 的输入列表（同样允许 plugin 扩展）。
	bindings []runtime.BindingResolver

	// fallbackUsage 记录“写入全局 registry 的兼容入口来源”，用于启动期 warning 与诊断。
	fallbackUsage map[string]int
}

// GlobalAOPRegistration 汇总“全局 AOP 声明”的快照。
//
// application 或 factory 写入的全局 AOP 最终都会先落在这里，
// 再由 compiler 合并进各个 handler 的执行计划。
type GlobalAOPRegistration struct {
	Middlewares  []aop.MiddlewareFunc
	Guards       []aop.GuardFunc
	Interceptors []aop.InterceptorFunc
	Pipes        []aop.PipeFunc
	Filters      []aop.ExceptionFilterFunc
}

// New 创建一个新的 Registry（编译期声明仓库）。
//
// Registry 在 v3 中承担“单一事实来源”的角色：协议插件把声明写进来，
// compiler 在编译阶段从这里读取声明并生成运行时可用的路由/matcher/AOP/binding 产物。
func New() *Registry {
	return &Registry{
		decls:         make(map[string]map[string][]any),
		bindings:      make([]runtime.BindingResolver, 0),
		fallbackUsage: make(map[string]int),
	}
}

var global = New()

// Global 返回全局单例 Registry。
//
// 应用层 DSL/装饰器通常向 Global() 写入声明，随后 application/compiler 从这里汇总并编译。
func Global() *Registry { return global }

// GlobalWithUsage 返回全局 Registry，并记录一次兼容入口的使用来源。
//
// 该方法只用于“真正写入全局 registry 的兼容路径”打点，例如包级 DSL / NewCompatAPI()。
func GlobalWithUsage(source string) *Registry {
	global.RecordFallbackUsage(source)
	return global
}

// ResetGlobal 重置全局单例 Registry（主要用于测试隔离或热重载场景）。
func ResetGlobal() { global = New() }

// RecordFallbackUsage 记录一次全局 registry 兼容入口使用。
func (r *Registry) RecordFallbackUsage(source string) {
	if r == nil || source == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fallbackUsage == nil {
		r.fallbackUsage = make(map[string]int)
	}
	r.fallbackUsage[source]++
}

// SnapshotFallbackUsage 返回兼容入口来源计数的快照。
// 它主要服务启动报告与诊断，帮助定位“哪里还在依赖全局 registry 兼容入口”。
func (r *Registry) SnapshotFallbackUsage() map[string]int {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.fallbackUsage) == 0 {
		return nil
	}
	out := make(map[string]int, len(r.fallbackUsage))
	maps.Copy(out, r.fallbackUsage)
	return out
}

// RegisterDecl 为某个插件注册声明。
// 这是协议插件最核心的可插拔写入入口。
//
// 约定：
// - pluginID：插件标识（例如 "http" / "grpc"）
// - kind：声明种类（由插件定义，例如 "routes" / "schemas"）
// - items：声明对象列表（结构由插件自行定义）
func (r *Registry) RegisterDecl(pluginID, kind string, items ...any) {
	if pluginID == "" || kind == "" || len(items) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	kinds := r.decls[pluginID]
	if kinds == nil {
		kinds = make(map[string][]any)
		r.decls[pluginID] = kinds
	}
	kinds[kind] = append(kinds[kind], items...)
}

// ListDecl 返回某个插件某类声明的快照。
//
// 返回的是切片快照（copy），避免调用方修改内部状态。
func (r *Registry) ListDecl(pluginID, kind string) []any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	kinds := r.decls[pluginID]
	if kinds == nil {
		return nil
	}
	items := kinds[kind]
	if len(items) == 0 {
		return nil
	}
	out := make([]any, len(items))
	copy(out, items)
	return out
}

// SnapshotDeclCounts 返回 `pluginID -> kind -> count` 的稳定快照。
// 主要供编译诊断使用。
func (r *Registry) SnapshotDeclCounts() map[string]map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(map[string]map[string]int, len(r.decls))
	for pluginID, kinds := range r.decls {
		if len(kinds) == 0 {
			continue
		}
		kindCounts := make(map[string]int, len(kinds))
		for kind, items := range kinds {
			kindCounts[kind] = len(items)
		}
		out[pluginID] = kindCounts
	}
	return out
}

// UseGlobalMiddlewares 追加全局 middlewares 声明。
//
// compiler 会把这些全局 AOP 展开到每个 handler 的 AOP 计划中（或参与运行期合并，视实现而定）。
func (r *Registry) UseGlobalMiddlewares(items ...aop.MiddlewareFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.globalAOP.Middlewares = append(r.globalAOP.Middlewares, items...)
}

// UseGlobalGuards 追加全局 guards 声明。
func (r *Registry) UseGlobalGuards(items ...aop.GuardFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.globalAOP.Guards = append(r.globalAOP.Guards, items...)
}

// UseGlobalInterceptors 追加全局 interceptors 声明。
func (r *Registry) UseGlobalInterceptors(items ...aop.InterceptorFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.globalAOP.Interceptors = append(r.globalAOP.Interceptors, items...)
}

// UseGlobalPipes 追加全局 pipes 声明。
func (r *Registry) UseGlobalPipes(items ...aop.PipeFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.globalAOP.Pipes = append(r.globalAOP.Pipes, items...)
}

// UseGlobalFilters 追加全局 exception filters 声明。
func (r *Registry) UseGlobalFilters(items ...aop.ExceptionFilterFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.globalAOP.Filters = append(r.globalAOP.Filters, items...)
}

// GetGlobalAOP 返回全局 AOP 声明的快照（切片均为拷贝）。
func (r *Registry) GetGlobalAOP() GlobalAOPRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return GlobalAOPRegistration{
		Middlewares:  append([]aop.MiddlewareFunc{}, r.globalAOP.Middlewares...),
		Guards:       append([]aop.GuardFunc{}, r.globalAOP.Guards...),
		Interceptors: append([]aop.InterceptorFunc{}, r.globalAOP.Interceptors...),
		Pipes:        append([]aop.PipeFunc{}, r.globalAOP.Pipes...),
		Filters:      append([]aop.ExceptionFilterFunc{}, r.globalAOP.Filters...),
	}
}

// RegisterBindings 注册一组 BindingResolver 声明。
//
// compiler 在编译阶段会把这些 resolver 组装进运行时的 BindingRegistry。
func (r *Registry) RegisterBindings(resolvers ...runtime.BindingResolver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bindings = append(r.bindings, resolvers...)
}

// GetBindings 返回当前已注册的 bindings 快照。
// 返回副本的原因和 declarations 一样，都是避免外部改动内部状态。
func (r *Registry) GetBindings() []runtime.BindingResolver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]runtime.BindingResolver, len(r.bindings))
	copy(out, r.bindings)
	return out
}
