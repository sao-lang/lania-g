// container.go 实现 v3 的轻量依赖注入容器。
//
// 它支撑的核心能力有三类：
// - provider 注册与 token 解析
// - singleton/request/transient 三种作用域缓存
// - parent/child 容器链，用于请求级隔离
package di

import (
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/sao-lang/lania-g/kernel/v3/metadata"
)

// Scope 复用 metadata 中定义的作用域枚举。
type Scope = metadata.Scope

const (
	// Singleton 表示实例缓存在“定义该 provider 的 container”中，整个进程生命周期复用。
	Singleton = metadata.ScopeSingleton
	// Request 表示实例缓存在“请求级 container”中（通常是 root.NewChild()），每个请求一份。
	Request   = metadata.ScopeRequest
	// Transient 表示不缓存，每次 Get 都会重新创建。
	Transient = metadata.ScopeTransient
)

// ProviderType 表示一个 provider 的创建方式。
type ProviderType string

const (
	// ProviderTypeClass 用反射构造一个 struct（支持 constructor finder 或 Injectable.Inject）。
	ProviderTypeClass    ProviderType = "class"
	// ProviderTypeValue 直接返回常量值（通常单例）。
	ProviderTypeValue    ProviderType = "value"
	// ProviderTypeFactory 通过工厂函数创建（可配合 Scope 做缓存）。
	ProviderTypeFactory  ProviderType = "factory"
	// ProviderTypeExisting 将 token 代理到另一个 token（alias）。
	ProviderTypeExisting ProviderType = "existing"
)

var (
	// ErrProviderNotFound 表示当前容器链上找不到对应 token 的 provider。
	ErrProviderNotFound   = errors.New("provider not found")
	// ErrCircularDependency 表示解析依赖时出现循环依赖。
	ErrCircularDependency = errors.New("circular dependency detected")
)

// Provider 描述一个可注入的依赖项（类似 NestJS provider）。
//
// 约定：
// - Token 可以是任意可比较的 key（常见为 reflect.Type 或自定义 struct/string）
// - Scope 决定实例缓存落在哪个 container（见 cacheContainerFor）
type Provider struct {
	Token       interface{}
	Type        ProviderType
	UseClass    reflect.Type
	UseValue    interface{}
	UseFactory  func() (interface{}, error)
	UseExisting interface{}
	Scope       Scope
	Deps        []interface{}
}

// ConstructorFinder 用于为某个 struct 类型选择一个构造函数。
//
// 返回 `(constructor, true)` 表示命中；否则容器会回退到默认构造流程。
type ConstructorFinder func(typ reflect.Type) (reflect.Value, bool)

// Container 是 v3 的轻量 DI 容器，支持：
// - 父子容器：child 会向 parent 回溯查找 provider
// - Scope：Singleton/Request/Transient 三种实例生命周期
// - 循环依赖检测：通过 resolving map 记录当前解析栈
type Container struct {
	providers         map[interface{}]*Provider
	instances         map[interface{}]interface{}
	resolving         map[interface{}]bool
	mu                sync.RWMutex
	parent            *Container
	constructorFinder ConstructorFinder
}

// NewContainer 创建一个新的 DI 容器实例。
//
// 默认行为：
// - providers/instances/resolving 都初始化为空 map
// - constructorFinder 默认 nil，表示只走“默认构造 + Injectable.Inject”路径
func NewContainer() *Container {
	return &Container{
		providers:         make(map[interface{}]*Provider),
		instances:         make(map[interface{}]interface{}),
		resolving:         make(map[interface{}]bool),
		constructorFinder: nil,
	}
}

// SetConstructorFinder 设置“构造函数查找器”。
//
// 该 finder 用于给某些 struct 类型选择一个构造函数（constructor）来做构造注入，
// 以替代默认的 `reflect.New(T)` + `Injectable.Inject` 路径。
func (c *Container) SetConstructorFinder(finder ConstructorFinder) {
	c.constructorFinder = finder
}

// SetParent 设置父容器。
//
// 子容器在解析 token 时，如果当前容器没有 provider，会向 parent 回溯查找。
func (c *Container) SetParent(parent *Container) {
	c.parent = parent
}

// NewChild 创建一个子容器（child container）。
//
// 语义：
// - child 继承 parent 的 constructorFinder
// - child 的 providers/instances 与 parent 隔离
// - 解析时 child 会回溯 parent 查找 provider
//
// 常见用途：作为 request-scope 容器，每个请求创建一个 child，避免 request 依赖跨请求复用。
func (c *Container) NewChild() *Container {
	child := NewContainer()
	child.parent = c
	child.constructorFinder = c.constructorFinder
	return child
}

// Register 在当前容器注册一个 Provider 定义（token -> provider）。
//
// 注意：
// - 这里只注册“如何创建实例”的定义，不会立即创建实例
// - 同一个 token 重复注册会覆盖旧定义（后注册覆盖前注册）
func (c *Container) Register(provider *Provider) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.providers[provider.Token] = provider
}

// Get 从当前容器（向上回溯 parent）解析 token 对应的实例。
//
// requester 用于 Request scope 的缓存：通常传入“当前请求的 child container”，
// 使得 request scope 的实例落在该 child 上，而不是落在 provider 定义所在的 container 上。
func (c *Container) Get(token interface{}) (interface{}, error) {
	return c.get(token, make(map[interface{}]bool), c)
}

// RegisterClass 便捷注册一个“Class provider”：
// - token：依赖标识
// - typ：要构造的类型（struct 或 *struct）
// - scope：实例生命周期（Singleton/Request/Transient）
func (c *Container) RegisterClass(token interface{}, typ reflect.Type, scope Scope) {
	c.Register(&Provider{
		Token:    token,
		Type:     ProviderTypeClass,
		UseClass: typ,
		Scope:    scope,
	})
}

// RegisterValue 便捷注册一个“Value provider”（常量值）。
//
// 约定：Value provider 默认是 Singleton。
func (c *Container) RegisterValue(token interface{}, value interface{}) {
	c.Register(&Provider{
		Token:    token,
		Type:     ProviderTypeValue,
		UseValue: value,
		Scope:    Singleton,
	})
}

// RegisterFactory 便捷注册一个“Factory provider”。
//
// factory 会在首次创建（或每次创建，取决于 scope）时调用产生实例。
func (c *Container) RegisterFactory(token interface{}, factory func() (interface{}, error), scope Scope) {
	c.Register(&Provider{
		Token:      token,
		Type:       ProviderTypeFactory,
		UseFactory: factory,
		Scope:      scope,
	})
}

// RegisterExisting 便捷注册一个“Existing provider”（别名/代理）。
//
// 语义：解析 token 时，实际解析 existing token 的实例。
func (c *Container) RegisterExisting(token interface{}, existing interface{}) {
	c.Register(&Provider{
		Token:       token,
		Type:        ProviderTypeExisting,
		UseExisting: existing,
		Scope:       Singleton,
	})
}

// get 是 Get 的内部实现，增加了：
// - resolving：用于循环依赖检测的“解析栈”
// - requester：请求级容器，用于 Request scope 的缓存定位（见 cacheContainerFor）
func (c *Container) get(token interface{}, resolving map[interface{}]bool, requester *Container) (interface{}, error) {
	c.mu.RLock()
	provider, exists := c.providers[token]
	if !exists {
		if c.parent != nil {
			// provider 定义允许沿 parent 链回溯查找；
			// 这样 child container 只需要覆盖少量 request-scope provider。
			c.mu.RUnlock()
			return c.parent.get(token, resolving, requester)
		}
		c.mu.RUnlock()
		return nil, fmt.Errorf("%w: %v", ErrProviderNotFound, token)
	}

	cacheContainer := c.cacheContainerFor(provider.Scope, requester)
	if cacheContainer != nil {
		if instance, ok := cacheContainer.getCached(token); ok {
			c.mu.RUnlock()
			return instance, nil
		}
	}

	c.mu.RUnlock()

	if resolving[token] {
		// 循环依赖检测只关心当前解析栈，不和缓存状态混在一起。
		return nil, ErrCircularDependency
	}
	resolving[token] = true
	defer delete(resolving, token)

	if cacheContainer == nil {
		return c.createInstance(provider, resolving, requester)
	}

	cacheContainer.mu.Lock()
	if instance, ok := cacheContainer.instances[token]; ok {
		cacheContainer.mu.Unlock()
		return instance, nil
	}

	instance, err := c.createInstance(provider, resolving, requester)
	if err != nil {
		cacheContainer.mu.Unlock()
		return nil, err
	}

	cacheContainer.instances[token] = instance
	cacheContainer.mu.Unlock()

	return instance, nil
}

// createInstance 根据 provider.Type 创建实例（不负责缓存写入）。
//
// 注意：
// - Value：直接返回 UseValue
// - Existing：递归解析 UseExisting（仍会参与循环依赖检测）
// - Factory：调用 UseFactory
// - Class：调用 construct 做构造注入/Injectable 注入
func (c *Container) createInstance(provider *Provider, resolving map[interface{}]bool, requester *Container) (interface{}, error) {
	switch provider.Type {
	case ProviderTypeValue:
		instance := provider.UseValue
		if err := c.injectFields(instance, resolving, requester); err != nil {
			return nil, err
		}
		return instance, nil

	case ProviderTypeExisting:
		if resolving[provider.UseExisting] {
			return nil, ErrCircularDependency
		}
		return c.get(provider.UseExisting, resolving, requester)

	case ProviderTypeFactory:
		return provider.UseFactory()

	case ProviderTypeClass:
		return c.construct(provider.UseClass, resolving, requester)

	default:
		return nil, fmt.Errorf("unknown provider type: %v", provider.Type)
	}
}

// construct 构造一个 struct 实例，并执行注入逻辑。
//
// 构造流程：
// 1) typ 若是指针则取 Elem，确保构造的是 struct
// 2) 如果设置了 constructorFinder 且找到 constructor，则走 callConstructor（构造函数注入）
// 3) 否则 reflect.New(typ) 得到 *T
// 4) 若实例实现 Injectable，则调用 Injectable.Inject(container)
// 5) 预留 injectFields（当前 v3 不做基于 tag/装饰器的字段注入）
func (c *Container) construct(typ reflect.Type, resolving map[interface{}]bool, requester *Container) (interface{}, error) {
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}

	if typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("can only construct struct types")
	}

	if c.constructorFinder != nil {
		constructor, ok := c.constructorFinder(typ)
		if ok && constructor.IsValid() {
			// 一旦 finder 命中，就以构造函数注入为最高优先级，不再走默认 new + Injectable。
			return c.callConstructor(constructor, resolving, requester)
		}
	}

	value := reflect.New(typ)
	instance := value.Interface()

	if injectable, ok := instance.(Injectable); ok {
		// Injectable 适合那些不想暴露构造函数、但又需要容器参与初始化的类型。
		if err := injectable.Inject(c); err != nil {
			return nil, err
		}
	}

	if err := c.injectFields(instance, resolving, requester); err != nil {
		return nil, err
	}

	return instance, nil
}

// injectFields 对 struct 的 exported 零值字段按类型从容器注入实例。
//
// 规则：
// - 只注入 exported（首字母大写）字段
// - 只注入零值字段（已有值的字段不覆盖）
// - 容器中不存在的类型静默跳过（不报错）
// - 支持 parent 链回溯（child container 可注入 parent 的 provider）
func (c *Container) injectFields(instance interface{}, resolving map[interface{}]bool, requester *Container) error {
	rv := reflect.ValueOf(instance)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}

	for i := 0; i < rv.NumField(); i++ {
		ft := rv.Type().Field(i)
		fv := rv.Field(i)

		if !ft.IsExported() || !fv.IsZero() {
			continue
		}

		dep, err := c.get(ft.Type, resolving, requester)
		if err != nil {
			continue
		}

		fv.Set(reflect.ValueOf(dep))
	}
	return nil
}

// callConstructor 调用构造函数并解析其入参依赖。
//
// 约定：
// - constructor 的每个入参都被视为一个依赖 token（这里直接用 reflect.Type 作为 token）
// - constructor 必须至少返回一个值（作为实例）
// - 如果第二个返回值存在且为 error，会作为构造错误返回
func (c *Container) callConstructor(constructor reflect.Value, resolving map[interface{}]bool, requester *Container) (interface{}, error) {
	constructorType := constructor.Type()
	numParams := constructorType.NumIn()
	args := make([]reflect.Value, numParams)

	for i := 0; i < numParams; i++ {
		paramType := constructorType.In(i)
		// 构造函数参数直接把 `reflect.Type` 当作 token 解析，
		// 这和 module/runtime 里广泛使用 type token 的方式保持一致。
		dep, err := c.get(paramType, resolving, requester)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve dependency for parameter %d: %w", i, err)
		}
		args[i] = reflect.ValueOf(dep)
	}

	results := constructor.Call(args)
	if len(results) == 0 {
		return nil, fmt.Errorf("constructor must return at least one value")
	}

	instance := results[0].Interface()

	var err error
	if len(results) > 1 && !results[1].IsNil() {
		if e, ok := results[1].Interface().(error); ok {
			err = e
		}
	}

	return instance, err
}

// cacheContainerFor 决定某个 scope 的实例应该缓存在哪个容器上。
//
// 规则：
// - Singleton：缓存到“provider 定义所在容器”（当前 c）
// - Request：缓存到 requester（通常是 per-request child）；若 requester=nil，则退化为缓存到 c
// - Transient：不缓存（返回 nil）
func (c *Container) cacheContainerFor(scope Scope, requester *Container) *Container {
	switch scope {
	case Singleton:
		return c
	case Request:
		if requester != nil {
			return requester
		}
		return c
	default:
		return nil
	}
}

// getCached 从当前容器读取已缓存的实例（线程安全）。
func (c *Container) getCached(token interface{}) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	instance, ok := c.instances[token]
	return instance, ok
}

// GetAll 返回当前容器及其 parent 链上的所有 Provider 定义（快照）。
//
// 合并规则：
// - 以“更靠近当前容器”的 provider 为准（child 覆盖 parent）
// - 返回的是新的 map，避免外部修改内部状态
func (c *Container) GetAll() map[interface{}]*Provider {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[interface{}]*Provider)
	for k, v := range c.providers {
		result[k] = v
	}

	if c.parent != nil {
		parentProviders := c.parent.GetAll()
		for k, v := range parentProviders {
			if _, exists := result[k]; !exists {
				result[k] = v
			}
		}
	}

	return result
}

// Injectable 允许实例在默认构造完成后自行执行注入逻辑。
//
// 这适合那些不想显式声明构造函数、但又需要在实例创建后拿到容器的类型。
type Injectable interface {
	Inject(container *Container) error
}

// Get 是泛型版的 Container.Get：会在取回实例后做类型断言。
//
// 注意：
// - token 仍由调用方传入（可为 reflect.Type 或其他自定义 token）
// - 断言失败会返回错误而不是 panic
func Get[T any](c *Container, token interface{}) (T, error) {
	instance, err := c.Get(token)
	if err != nil {
		var zero T
		return zero, err
	}
	result, ok := instance.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("type assertion failed: expected %T, got %T", zero, instance)
	}
	return result, nil
}

// MustGet 是 Get 的 panic 版本：解析或断言失败会 panic。
func MustGet[T any](c *Container, token interface{}) T {
	result, err := Get[T](c, token)
	if err != nil {
		panic(err)
	}
	return result
}

// RegisterClass 使用泛型类型参数推导要注册的 class 类型，并注册为 Class provider。
//
// 注意：这里用 `var zero T` + reflect.TypeOf(zero) 推导类型，
// 因此若 T 是接口，typ 可能为 nil；建议在上层使用 reflect.Type token 更明确地注册。
func RegisterClass[T any](c *Container, token interface{}, scope Scope) {
	var zero T
	typ := reflect.TypeOf(zero)
	c.RegisterClass(token, typ, scope)
}

// RegisterValue 注册一个 value provider（泛型版）。
func RegisterValue[T any](c *Container, token interface{}, value T) {
	c.RegisterValue(token, value)
}

// RegisterFactory 注册一个 factory provider（泛型版），并把 factory 的返回值适配为 interface{}。
func RegisterFactory[T any](c *Container, token interface{}, factory func() (T, error), scope Scope) {
	c.RegisterFactory(token, func() (interface{}, error) {
		return factory()
	}, scope)
}

// RegisterExisting 注册一个 existing provider（泛型版）。
func RegisterExisting[T any](c *Container, token interface{}, existing interface{}) {
	c.RegisterExisting(token, existing)
}

// ProviderFromInstance 将一个实例转换为 Provider 描述。
//
// 注意：这里的语义是“Class provider”：token 为实例类型，UseClass 为该类型。
// 如果你希望以常量值方式注册实例，应使用 ProviderFromInstanceWithToken。
func ProviderFromInstance[T any](instance T, scope Scope) (*Provider, error) {
	typ := reflect.TypeOf(instance)
	return &Provider{
		Token:    typ,
		Type:     ProviderTypeClass,
		UseClass: typ,
		Scope:    scope,
	}, nil
}

// ProviderFromInstanceWithToken 将一个实例转换为 Value provider，并允许自定义 token。
func ProviderFromInstanceWithToken[T any](token interface{}, instance T, scope Scope) (*Provider, error) {
	return &Provider{
		Token:    token,
		Type:     ProviderTypeValue,
		UseValue: instance,
		Scope:    scope,
	}, nil
}

// ProviderFromFactory 将一个泛型 factory 转换为 Factory provider。
func ProviderFromFactory[T any](token interface{}, factory func() (T, error), scope Scope) (*Provider, error) {
	return &Provider{
		Token: token,
		Type:  ProviderTypeFactory,
		UseFactory: func() (interface{}, error) {
			return factory()
		},
		Scope: scope,
	}, nil
}
