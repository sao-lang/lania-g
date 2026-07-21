// module.go 定义模块系统的核心抽象、默认实现和动态模块工厂约定。
//
// 这一层回答三个问题：
// - 一个模块最少要声明什么元信息
// - imports/exports 如何在模块容器之间传播
// - 动态模块、全局模块、生命周期钩子如何用统一接口表达
package module

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/di"
)

// ModuleMetadata 描述一个模块的声明信息。
//
// 其中：
// - Imports 表示模块依赖
// - Controllers/Resolvers 表示协议入口接收者
// - Providers 表示可注入依赖
// - Exports 表示可供其他模块复用的导出 token
type ModuleMetadata struct {
	Imports     []Module
	Controllers []interface{}
	Resolvers   []interface{}
	Providers   []*di.Provider
	Exports     []interface{}
	IsGlobal    bool
}

// Module 是 v3 的模块接口：每个模块自带一个 DI container，并通过 Metadata 描述 imports/providers/controllers 等。
// Init/Destroy 则负责把这份“声明”真正变成可运行的容器状态和生命周期回调。
type Module interface {
	Metadata() *ModuleMetadata
	Container() *di.Container
	Init() error
	Destroy() error
}

// BaseModule 是默认实现：支持 imports 的导出注入（Exports），并在 Init/Destroy 中触发生命周期回调。
// 大多数通过 `CreateModule()` 创建的普通模块，最终都是这个类型。
type BaseModule struct {
	metadata    *ModuleMetadata
	container   *di.Container
	initialized bool
}

// NewBaseModule 创建一个 BaseModule，并为其创建独立的 DI 容器。
//
// 每个模块拥有自己的 container：
// - providers/controllers/resolvers 会注册到该容器
// - imports 导出的 token 会在 Init 时“注入”到该容器（类似 re-export）
func NewBaseModule(metadata *ModuleMetadata) *BaseModule {
	container := di.NewContainer()
	return &BaseModule{
		metadata:  metadata,
		container: container,
	}
}

// Metadata 返回模块的元信息声明（imports/providers/controllers/resolvers/exports）。
func (m *BaseModule) Metadata() *ModuleMetadata {
	return m.metadata
}

// Container 返回模块自己的 DI 容器。
func (m *BaseModule) Container() *di.Container {
	return m.container
}

// Init 初始化模块（幂等）。
//
// 初始化内容：
// 1) 递归 Init imports
// 2) 将 imports 的 exports 解析为实例，并 RegisterValue 到当前模块容器（实现“导入可用”）
// 3) 注册本模块 providers
// 4) 将 controllers/resolvers 作为 value 注册到容器（让编译器/runtime 可按类型解析 receiver）
// 5) 对所有非 Request scope 的实例触发 OnModuleInit（如果实现）
func (m *BaseModule) Init() error {
	if m.initialized {
		return nil
	}

	// 先 Init imports，再把 imports 导出的 token 实例注册到当前模块容器中。
	// 这样当前模块里的 provider/controller 可以像访问本地 provider 一样访问 import 导出。
	for _, imp := range m.metadata.Imports {
		if err := imp.Init(); err != nil {
			return err
		}

		for _, exportToken := range exportedTokens(imp) {
			impContainer := imp.Container()
			instance, err := impContainer.Get(exportToken)
			if err != nil {
				continue
			}

			m.container.RegisterValue(exportToken, instance)
		}
	}

	for _, provider := range m.metadata.Providers {
		m.container.Register(provider)
	}

	// Controllers/Resolvers 也需要纳入容器管理，
	// 这样 compiler/runtime 就能按类型解析 receiver 的模块归属，
	// 而不是依赖 DSL 里传入的裸实例。
	//
	// 每个实例先走 tryConstructOwner 尝试依赖注入（字段注入或构造函数注入）：
	// - 若实例所有字段都是零值 → 走 construct 管线（reflect.New + injectFields / ConstructorFinder）
	// - 若实例已有非零字段（老写法）→ 保持原实例不变
	for _, controller := range m.metadata.Controllers {
		if controller == nil {
			continue
		}
		instance := m.tryInjectOwner(controller)
		t := reflect.TypeOf(instance)
		if t.Kind() != reflect.Ptr {
			t = reflect.PointerTo(t)
		}
		m.container.RegisterValue(t, instance)
	}
	for _, resolver := range m.metadata.Resolvers {
		if resolver == nil {
			continue
		}
		instance := m.tryInjectOwner(resolver)
		t := reflect.TypeOf(instance)
		if t.Kind() != reflect.Ptr {
			t = reflect.PointerTo(t)
		}
		m.container.RegisterValue(t, instance)
	}

	providers := m.container.GetAll()
	for token := range providers {
		if provider := providers[token]; provider != nil && provider.Scope == di.Request {
			// Request scope 实例不应在模块初始化阶段提前创建。
			continue
		}
		instance, err := m.container.Get(token)
		if err != nil {
			continue
		}

		if initable, ok := instance.(OnModuleInit); ok {
			if err := initable.OnModuleInit(); err != nil {
				return err
			}
		}
	}

	m.initialized = true
	return nil
}

// tryInjectOwner 尝试对 controller/resolver 实例做依赖注入。
//
// 检测策略：
// - 若实例的所有 exported 字段都是零值 → 认为用户没手动赋值，走 Class provider 构造路径
//   → construct() → 优先 ConstructorFinder（构造函数注入）→ 否则 reflect.New + injectFields
// - 若有任一 exported 字段非零（老写法手动赋值）→ 保持原实例不变
//
// 这样既兼容老代码（&Controller{store: s}），又支持新写法（&Controller{} 自动注入字段）。
func (m *BaseModule) tryInjectOwner(owner interface{}) interface{} {
	if owner == nil {
		return nil
	}

	// 检查是否所有 exported 字段都是零值
	rv := reflect.ValueOf(owner)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return owner
	}

	allZero := true
	for i := 0; i < rv.NumField(); i++ {
		if rv.Type().Field(i).IsExported() && !rv.Field(i).IsZero() {
			allZero = false
			break
		}
	}
	if !allZero {
		return owner
	}

	// 全零值 → 走 Class provider 构造路径
	provider := &di.Provider{
		Token:    reflect.TypeOf(owner),
		Type:     di.ProviderTypeClass,
		UseClass: reflect.TypeOf(owner),
		Scope:    di.Singleton,
	}
	instance, err := m.container.Get(provider.Token)
	if err != nil {
		return owner
	}
	return instance
}

// Destroy 销毁模块（逆序处理 imports），用于应用退出或动态卸载。
//
// 行为：
// - 对所有非 Request scope 实例触发 OnModuleDestroy（如果实现）
// - 逆序 Destroy imports（后 import 的模块先销毁）
//
// 注意：这里的错误处理偏“尽力而为”，会打印错误但不阻止后续销毁流程。
func (m *BaseModule) Destroy() error {
	providers := m.container.GetAll()
	for token := range providers {
		if provider := providers[token]; provider != nil && provider.Scope == di.Request {
			continue
		}
		instance, err := m.container.Get(token)
		if err != nil {
			continue
		}

		if destroyable, ok := instance.(OnModuleDestroy); ok {
			if err := destroyable.OnModuleDestroy(); err != nil {
				fmt.Printf("Error destroying module: %v\n", err)
			}
		}
	}

	for i := len(m.metadata.Imports) - 1; i >= 0; i-- {
		if err := m.metadata.Imports[i].Destroy(); err != nil {
			fmt.Printf("Error destroying imported module: %v\n", err)
		}
	}

	m.initialized = false
	return nil
}

// DynamicModule 表示动态创建得到的模块类型。
type DynamicModule interface {
	Module
}

// DynamicModuleFactory 定义一组“动态模块工厂”惯例接口。
// 它主要用于对齐常见的 ForRoot/ForFeature/RegisterAsync 等模块创建风格。
type DynamicModuleFactory interface {
	Register(options interface{}) (Module, error)
	ForRoot(options interface{}) (Module, error)
	ForFeature(options interface{}) (Module, error)
	RegisterAsync(options AsyncOptions) (Module, error)
	ForRootAsync(options AsyncOptions) (Module, error)
}

// AsyncOptions 描述动态模块异步注册时使用的参数。
type AsyncOptions struct {
	Imports    []Module
	UseFactory func() (interface{}, error)
	Inject     []interface{}
}

// GlobalModule 表示一个会自动注入到所有模块 imports 中的全局模块。
type GlobalModule struct {
	*BaseModule
}

// NewGlobalModule 创建一个全局模块（metadata.IsGlobal=true）。
//
// 全局模块会在 ModuleLoader.prepareGlobalModules 阶段被注入到所有模块 imports 中，
// 从而无需用户显式 import。
func NewGlobalModule(metadata *ModuleMetadata) *GlobalModule {
	metadata.IsGlobal = true
	return &GlobalModule{
		BaseModule: NewBaseModule(metadata),
	}
}

// ModuleDecorator 模拟“模块装饰器”风格的声明入口。
//
// 目前实现会忽略 target，仅返回一个根据 metadata 构建的 BaseModule，
// 主要用于保持 API 风格一致（对齐 NestJS 的 decorator 体验）。
func ModuleDecorator(metadata *ModuleMetadata) func(interface{}) Module {
	return func(target interface{}) Module {
		module := NewBaseModule(metadata)
		return module
	}
}

// Global 返回一个创建全局模块的“装饰器函数”。
//
// 同 ModuleDecorator，目前实现同样不依赖 target。
func Global() func(interface{}) Module {
	return func(target interface{}) Module {
		metadata := &ModuleMetadata{
			IsGlobal: true,
		}
		return NewGlobalModule(metadata)
	}
}

// ForwardRef 表示一个延迟求值的模块引用。
type ForwardRef func() Module

// NewForwardRef 创建一个延迟求值的模块引用。
// 它通常用于打破模块声明阶段的循环引用。
func NewForwardRef(fn func() Module) ForwardRef {
	return fn
}

// CreateModule 便捷创建一个普通模块（BaseModule）。
//
// 这是 v3 推荐的声明方式之一：显式传入 imports/providers/controllers/resolvers/exports。
func CreateModule(imports []Module, providers []*di.Provider, controllers []interface{}, resolvers []interface{}, exports []interface{}) Module {
	return NewBaseModule(&ModuleMetadata{
		Imports:     imports,
		Providers:   providers,
		Controllers: controllers,
		Resolvers:   resolvers,
		Exports:     exports,
	})
}

// CreateGlobalModule 便捷创建一个全局模块（GlobalModule）。
func CreateGlobalModule(imports []Module, providers []*di.Provider, controllers []interface{}, resolvers []interface{}, exports []interface{}) Module {
	return NewGlobalModule(&ModuleMetadata{
		Imports:     imports,
		Providers:   providers,
		Controllers: controllers,
		Resolvers:   resolvers,
		Exports:     exports,
	})
}

// ModuleReexport 描述从另一个模块转发导出 token 的声明。
type ModuleReexport struct {
	Module Module
	Tokens []interface{}
}

// Reexport 创建一个模块重导出声明。
func Reexport(module Module, tokens ...interface{}) *ModuleReexport {
	return &ModuleReexport{
		Module: module,
		Tokens: tokens,
	}
}

// exportedTokens 计算某个模块声明的 exports token 列表，并展开 ModuleReexport。
//
// 支持两种 export 形态：
// - 直接 export 某个 token（例如 reflect.Type 或自定义 token）
// - export *ModuleReexport：
//   - Tokens 为空：递归展开被 reexport 模块的 exports
//   - Tokens 非空：只 export 指定 tokens
func exportedTokens(mod Module) []interface{} {
	if mod == nil || mod.Metadata() == nil {
		return nil
	}
	out := make([]interface{}, 0, len(mod.Metadata().Exports))
	for _, item := range mod.Metadata().Exports {
		switch value := item.(type) {
		case *ModuleReexport:
			if value == nil || value.Module == nil {
				continue
			}
			if len(value.Tokens) == 0 {
				out = append(out, exportedTokens(value.Module)...)
				continue
			}
			out = append(out, value.Tokens...)
		default:
			out = append(out, value)
		}
	}
	return out
}

// OnModuleInit 定义模块初始化生命周期钩子。
type OnModuleInit interface {
	OnModuleInit() error
}

// OnModuleDestroy 定义模块销毁生命周期钩子。
type OnModuleDestroy interface {
	OnModuleDestroy() error
}

// OnApplicationBootstrap 定义应用启动完成后的生命周期钩子。
type OnApplicationBootstrap interface {
	OnApplicationBootstrap() error
}

// OnApplicationShutdown 定义应用关闭阶段的生命周期钩子。
type OnApplicationShutdown interface {
	OnApplicationShutdown() error
}

// DynamicModuleBase 是一个基于反射转发的动态模块工厂包装器。
// 它让用户无需显式实现整个 `DynamicModuleFactory` 接口，也能复用统一调用入口。
type DynamicModuleBase struct {
	factory interface{}
}

// NewDynamicModule 创建一个动态模块工厂包装器。
//
// factory 通常是用户自定义的 struct，提供 Register/ForRoot/ForFeature 等方法，
// DynamicModuleBase 会通过反射调用这些方法以统一动态模块 API。
func NewDynamicModule(factory interface{}) *DynamicModuleBase {
	return &DynamicModuleBase{
		factory: factory,
	}
}

// Register 通过反射调用 factory.Register(options)。
//
// 约定：Register 必须返回 (Module, error)。
func (d *DynamicModuleBase) Register(options interface{}) (Module, error) {
	method := reflect.ValueOf(d.factory).MethodByName("Register")
	if !method.IsValid() {
		return nil, fmt.Errorf("Register method not found")
	}

	results := method.Call([]reflect.Value{reflect.ValueOf(options)})
	if len(results) != 2 {
		return nil, fmt.Errorf("Register should return (Module, error)")
	}

	module, _ := results[0].Interface().(Module)
	err, _ := results[1].Interface().(error)
	return module, err
}

// ForRoot 通过反射调用 factory.ForRoot(options)。
//
// 约定：ForRoot 必须返回 (Module, error)。
func (d *DynamicModuleBase) ForRoot(options interface{}) (Module, error) {
	method := reflect.ValueOf(d.factory).MethodByName("ForRoot")
	if !method.IsValid() {
		return nil, fmt.Errorf("ForRoot method not found")
	}

	results := method.Call([]reflect.Value{reflect.ValueOf(options)})
	if len(results) != 2 {
		return nil, fmt.Errorf("ForRoot should return (Module, error)")
	}

	module, _ := results[0].Interface().(Module)
	err, _ := results[1].Interface().(error)
	return module, err
}

// ForFeature 通过反射调用 factory.ForFeature(options)。
//
// 约定：ForFeature 必须返回 (Module, error)。
func (d *DynamicModuleBase) ForFeature(options interface{}) (Module, error) {
	method := reflect.ValueOf(d.factory).MethodByName("ForFeature")
	if !method.IsValid() {
		return nil, fmt.Errorf("ForFeature method not found")
	}

	results := method.Call([]reflect.Value{reflect.ValueOf(options)})
	if len(results) != 2 {
		return nil, fmt.Errorf("ForFeature should return (Module, error)")
	}

	module, _ := results[0].Interface().(Module)
	err, _ := results[1].Interface().(error)
	return module, err
}

// RegisterAsync 通过反射调用 factory.RegisterAsync(options)。
//
// 约定：RegisterAsync 必须返回 (Module, error)。
func (d *DynamicModuleBase) RegisterAsync(options AsyncOptions) (Module, error) {
	method := reflect.ValueOf(d.factory).MethodByName("RegisterAsync")
	if !method.IsValid() {
		return nil, fmt.Errorf("RegisterAsync method not found")
	}

	results := method.Call([]reflect.Value{reflect.ValueOf(options)})
	if len(results) != 2 {
		return nil, fmt.Errorf("RegisterAsync should return (Module, error)")
	}

	module, _ := results[0].Interface().(Module)
	err, _ := results[1].Interface().(error)
	return module, err
}

// ForRootAsync 通过反射调用 factory.ForRootAsync(options)。
//
// 约定：ForRootAsync 必须返回 (Module, error)。
func (d *DynamicModuleBase) ForRootAsync(options AsyncOptions) (Module, error) {
	method := reflect.ValueOf(d.factory).MethodByName("ForRootAsync")
	if !method.IsValid() {
		return nil, fmt.Errorf("ForRootAsync method not found")
	}

	results := method.Call([]reflect.Value{reflect.ValueOf(options)})
	if len(results) != 2 {
		return nil, fmt.Errorf("ForRootAsync should return (Module, error)")
	}

	module, _ := results[0].Interface().(Module)
	err, _ := results[1].Interface().(error)
	return module, err
}

// CreateAutoModule 已在 v3 移除，保留该函数仅用于给出迁移提示。
func CreateAutoModule(imports []Module, exports []interface{}) (Module, error) {
	return nil, fmt.Errorf("CreateAutoModule is removed in v3: use CreateModule with explicit providers/controllers/resolvers")
}

// SimpleModule 已在 v3 移除，保留该函数仅用于给出迁移提示。
func SimpleModule() (Module, error) {
	return nil, fmt.Errorf("SimpleModule is removed in v3: use CreateModule")
}
