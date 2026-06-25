// binding.go 定义 runtime 的参数绑定抽象、注册表与缓存逻辑。
//
// 这层是各协议 binding 与 Executor 之间的桥：
// - 协议侧负责注册 resolver
// - Executor 只负责按参数类型查询并消费结果
package runtime

import (
	"fmt"
	"reflect"
	"sync"
)

// WrapperDescriptor 描述某个 BindingResolver 如何“命中”一个参数类型。
//
// 典型用法：
// - WrapperType/InnerType 用于描述诸如 `Param[T]` / `Body[T]` 这类包装类型
// - BindingName 用于命名绑定（例如取 path param/query/header 的 key）
// - ParamIndex 用于错误定位与诊断（由 Resolve 时注入）
type WrapperDescriptor struct {
	BindingName string
	Kind        string
	WrapperType reflect.Type
	InnerType   reflect.Type
	ParamIndex  int
}

// BindingRegistration 是一个便捷注册结构，适合在协议 adapter / integration 中声明 binding。
//
// 它把一个 resolver 拆成“名字/协议限制/类型匹配/值解析”四部分，便于配置式注册。
type BindingRegistration struct {
	Name             string
	AllowedProtocols map[Protocol]bool
	Match            func(t reflect.Type) (WrapperDescriptor, bool)
	Resolve          func(ctx *HandlerContext, desc WrapperDescriptor) (interface{}, error)
}

// NewBindingResolver 把声明式 BindingRegistration 转成真正可执行的 BindingResolver。
func NewBindingResolver(reg BindingRegistration) BindingResolver {
	return &registrationBindingResolver{registration: reg}
}

// NewBindingResolvers 批量把声明式 registration 转成 resolver 列表。
func NewBindingResolvers(regs ...BindingRegistration) []BindingResolver {
	if len(regs) == 0 {
		return nil
	}
	resolvers := make([]BindingResolver, 0, len(regs))
	for _, reg := range regs {
		resolvers = append(resolvers, NewBindingResolver(reg))
	}
	return resolvers
}

// BindingResolver 定义了“参数类型 -> 运行时值”的解析器。
// Executor.resolveArgument 会先走 binding，再回退到 DI（见 executor.go）。
type BindingResolver interface {
	Name() string
	AllowedProtocols() map[Protocol]bool
	Match(paramType reflect.Type) (WrapperDescriptor, bool)
	Resolve(ctx *HandlerContext, desc WrapperDescriptor) (interface{}, error)
}

// BindingRegistry 管理 binding resolvers 列表，并对 paramType -> (resolver, desc) 做缓存。
//
// 语义：
// - Resolver 匹配顺序是“后注册优先”（Find 从尾到头遍历），便于覆盖默认行为。
// - Register 会清空缓存，因为 resolver 顺序是语义的一部分。
type BindingRegistry struct {
	resolvers []BindingResolver
	mu        sync.RWMutex
	cache     map[reflect.Type]bindingCacheEntry
}

// NewBindingRegistry 创建一个 BindingRegistry。
//
// registry 负责：
// - 管理 resolver 注册顺序（后注册优先）
// - 缓存 paramType -> (resolver, descriptor, ok) 的查找结果，减少反射与遍历开销
func NewBindingRegistry() *BindingRegistry {
	return &BindingRegistry{
		resolvers: make([]BindingResolver, 0),
		cache:     make(map[reflect.Type]bindingCacheEntry),
	}
}

// Register 注册一个 BindingResolver。
//
// 语义：后注册优先（Find 从尾到头遍历），因此 Register 会影响匹配顺序。
// 为保证语义正确性，这里会清空缓存（否则缓存仍指向旧的 resolver 选择结果）。
func (br *BindingRegistry) Register(resolver BindingResolver) {
	br.mu.Lock()
	defer br.mu.Unlock()
	br.resolvers = append(br.resolvers, resolver)
	// resolver 的注册顺序会影响语义，因此这里需要清空缓存。
	if br.cache != nil {
		for k := range br.cache {
			delete(br.cache, k)
		}
	}
}

// RegisterBinding 使用 BindingRegistration 便捷注册一个 resolver。
//
// 适合在协议 adapter / integration 中用“配置式”方式声明 binding 规则。
func (br *BindingRegistry) RegisterBinding(reg BindingRegistration) {
	br.Register(&registrationBindingResolver{registration: reg})
}

// RegisterFunc 以函数方式注册一个简易 binding resolver。
//
// matcher 判断某个参数类型是否命中；resolve 负责返回该参数的运行时值。
// 该方式不会携带 wrapper/inner 的结构化信息（desc.Kind 固定为 "func"）。
func (br *BindingRegistry) RegisterFunc(matcher func(reflect.Type) bool, resolve func(ctx *HandlerContext, paramType reflect.Type) (interface{}, error)) {
	br.RegisterBinding(BindingRegistration{
		Name:             "func",
		AllowedProtocols: nil,
		Match: func(t reflect.Type) (WrapperDescriptor, bool) {
			if !matcher(t) {
				return WrapperDescriptor{}, false
			}
			return WrapperDescriptor{WrapperType: t, InnerType: t, Kind: "func"}, true
		},
		Resolve: func(ctx *HandlerContext, desc WrapperDescriptor) (interface{}, error) {
			return resolve(ctx, desc.WrapperType)
		},
	})
}

// bindingCacheEntry 是 BindingRegistry 内部的类型匹配缓存项。
type bindingCacheEntry struct {
	resolver BindingResolver
	desc     WrapperDescriptor
	ok       bool
}

// Find 查找某个 paramType 对应的 BindingResolver + WrapperDescriptor，并返回是否命中。
//
// 行为要点：
// - 先查缓存；未命中时遍历 resolvers（后注册优先）
// - 无论命中与否都会写入缓存（ok=false 的负缓存也会写入，减少重复遍历）
// - Find 只负责“类型匹配”，不处理协议限制与参数名覆盖（这些在 Resolve 中完成）
func (br *BindingRegistry) Find(paramType reflect.Type) (BindingResolver, WrapperDescriptor, bool) {
	br.mu.RLock()
	if br.cache != nil {
		if cached, ok := br.cache[paramType]; ok {
			br.mu.RUnlock()
			return cached.resolver, cached.desc, cached.ok
		}
	}

	// 后注册优先：越靠后的 resolver 越“具体”，允许覆盖默认实现。
	for i := len(br.resolvers) - 1; i >= 0; i-- {
		desc, ok := br.resolvers[i].Match(paramType)
		if ok {
			res := br.resolvers[i]
			br.mu.RUnlock()

			// 在写锁下回填缓存。
			br.mu.Lock()
			if br.cache == nil {
				br.cache = make(map[reflect.Type]bindingCacheEntry)
			}
			br.cache[paramType] = bindingCacheEntry{resolver: res, desc: desc, ok: true}
			br.mu.Unlock()
			return res, desc, true
		}
	}
	br.mu.RUnlock()

	br.mu.Lock()
	if br.cache == nil {
		br.cache = make(map[reflect.Type]bindingCacheEntry)
	}
	br.cache[paramType] = bindingCacheEntry{resolver: nil, desc: WrapperDescriptor{}, ok: false}
	br.mu.Unlock()
	return nil, WrapperDescriptor{}, false
}

// Resolve 将某个参数类型解析为运行时值。
//
// 解析流程：
// 1) Find(paramType) 选择 resolver 并拿到 descriptor（可能来自缓存）
// 2) 校验协议是否允许（AllowedProtocols）
// 3) 用 handler.Meta.ParamPlans[paramIndex].BindingName 覆盖 desc.BindingName（如果编译期指定了名字）
// 4) 调用 resolver.Resolve(ctx, desc) 产生最终值
func (br *BindingRegistry) Resolve(ctx *HandlerContext, handler *Handler, paramType reflect.Type, paramIndex int) (interface{}, error) {
	resolver, desc, ok := br.Find(paramType)
	if !ok {
		return nil, ErrBindingNotFound
	}

	desc.ParamIndex = paramIndex
	if allowed := resolver.AllowedProtocols(); len(allowed) > 0 && !allowed[ctx.Protocol] {
		return nil, fmt.Errorf("%w: protocol=%s binding=%s type=%s", ErrBindingNotSupported, ctx.Protocol, resolver.Name(), paramType.String())
	}

	if handler != nil && paramIndex >= 0 && paramIndex < len(handler.Meta.ParamPlans) {
		if name := handler.Meta.ParamPlans[paramIndex].BindingName; name != "" {
			// 编译期若已经为这个参数钉死 bindingName，则以编译结果为准覆盖 matcher 默认值。
			desc.BindingName = name
		}
	}

	return resolver.Resolve(ctx, desc)
}

type registrationBindingResolver struct {
	registration BindingRegistration
}

// Name 返回该 resolver 的名字（用于诊断与错误信息）。
func (r *registrationBindingResolver) Name() string { return r.registration.Name }

// AllowedProtocols 返回允许该 binding 生效的协议集合。
//
// 返回 nil/空 map 表示不限制协议；否则仅当 allowed[ctx.Protocol] 为 true 时允许使用。
func (r *registrationBindingResolver) AllowedProtocols() map[Protocol]bool {
	return r.registration.AllowedProtocols
}

// Match 判断某个参数类型是否命中该 binding，并返回 wrapper descriptor。
func (r *registrationBindingResolver) Match(paramType reflect.Type) (WrapperDescriptor, bool) {
	return r.registration.Match(paramType)
}

// Resolve 根据 descriptor 生成运行时值。
func (r *registrationBindingResolver) Resolve(ctx *HandlerContext, desc WrapperDescriptor) (interface{}, error) {
	return r.registration.Resolve(ctx, desc)
}
