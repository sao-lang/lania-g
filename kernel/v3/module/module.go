// module.go 定义模块系统的核心抽象与默认实现。
//
// 全仓只有两个模块构造入口：
//   - module.New(Metadata)      普通模块
//   - module.AsGlobal(Metadata) 全局模块（自动注入到所有模块的 imports）
//
// 模块是唯一的装配单元：imports / providers / controllers / exports / AOP
// 都写在 Metadata 里，路由等协议声明由控制器在所属模块的上下文内完成。
package module

import (
	"fmt"
	"reflect"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/di"
)

// AOP 汇总模块向应用贡献的全局 AOP。
//
// 它的语义等价于 NestJS 的 APP_GUARD / APP_INTERCEPTOR / APP_PIPE / APP_FILTER：
// 模块在装配阶段声明，应用在启动阶段按模块加载顺序合并。
type AOP struct {
	Middlewares  []aop.Middleware
	Guards       []aop.Guard
	Interceptors []aop.Interceptor
	Pipes        []aop.Pipe
	Filters      []aop.ExceptionFilter
}

// IsZero 判断当前 AOP 是否为空。
func (a AOP) IsZero() bool {
	return len(a.Middlewares) == 0 &&
		len(a.Guards) == 0 &&
		len(a.Interceptors) == 0 &&
		len(a.Pipes) == 0 &&
		len(a.Filters) == 0
}

// Metadata 描述一个模块的声明信息。
type Metadata struct {
	Imports     []Module
	Providers   []*di.Provider
	Controllers []any
	Resolvers   []any
	Exports     []any
	IsGlobal    bool
	AOP         AOP
}

// Module 是模块的统一抽象：每个模块自带一个 DI container，
// 并通过 Metadata 描述 imports/providers/controllers 等。
type Module interface {
	Metadata() *Metadata
	Container() *di.Container
	Init() error
	Destroy() error
}

// BaseModule 是 Module 的默认实现。
type BaseModule struct {
	metadata    *Metadata
	container   *di.Container
	context     *Context
	initialized bool
}

// New 创建一个普通模块。
//
// 每个模块拥有独立的 DI 容器：
//   - providers/controllers/resolvers 注册到该容器
//   - imports 导出的 token 在 Init 阶段注入到该容器（re-export）
func New(metadata Metadata) *BaseModule {
	return &BaseModule{
		metadata:  &metadata,
		container: di.NewContainer(),
	}
}

// GlobalModule 表示一个会自动注入到所有模块 imports 中的全局模块。
type GlobalModule struct {
	*BaseModule
}

// AsGlobal 创建一个全局模块（Metadata.IsGlobal = true）。
//
// 全局模块在 loader 加载阶段被注入到所有模块的 imports 中，无需用户显式 import。
func AsGlobal(metadata Metadata) *GlobalModule {
	metadata.IsGlobal = true
	return &GlobalModule{BaseModule: New(metadata)}
}

// Metadata 返回模块声明。
func (m *BaseModule) Metadata() *Metadata { return m.metadata }

// Container 返回模块自己的 DI 容器。
func (m *BaseModule) Container() *di.Container { return m.container }

// Context 返回模块上下文。
//
// 它在模块 Init 阶段可用，提供容器、声明归属与全局 AOP 贡献能力。
// 集成模块通过它注册 binding，无需依赖 registry 全局单例。
func (m *BaseModule) Context() *Context { return m.context }

// SetModuleContext 由 ModuleLoader 在 Init 前调用。
func (m *BaseModule) SetModuleContext(ctx *Context) { m.context = ctx }

// Init 初始化模块（幂等）。
//
// 初始化内容：
//  1. 递归 Init imports
//  2. 将 imports 的 exports 解析为实例并注册到当前容器（实现“导入可用”）
//  3. 注册本模块 providers
//  4. 将 controllers/resolvers 注册到容器（供编译器/运行期按类型解析 receiver）
//  5. 对所有非 Request scope 实例触发 OnModuleInit
func (m *BaseModule) Init() error {
	if m.initialized {
		return nil
	}

	for _, imp := range m.metadata.Imports {
		if err := imp.Init(); err != nil {
			return err
		}

		for _, exportToken := range exportedTokens(imp) {
			instance, err := imp.Container().Get(exportToken)
			if err != nil {
				continue
			}
			m.container.RegisterValue(exportToken, instance)
		}
	}

	for _, provider := range m.metadata.Providers {
		m.container.Register(provider)
	}

	for _, controller := range m.metadata.Controllers {
		if controller == nil {
			continue
		}
		instance := m.tryInjectOwner(controller)
		m.container.RegisterValue(pointerType(instance), instance)
	}
	for _, resolver := range m.metadata.Resolvers {
		if resolver == nil {
			continue
		}
		instance := m.tryInjectOwner(resolver)
		m.container.RegisterValue(pointerType(instance), instance)
	}

	providers := m.container.GetAll()
	for token := range providers {
		if provider := providers[token]; provider != nil && provider.Scope == di.Request {
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

// pointerType 返回实例的指针类型（若本身是指针则保持不变）。
func pointerType(instance any) reflect.Type {
	t := reflect.TypeOf(instance)
	if t == nil {
		return nil
	}
	if t.Kind() != reflect.Ptr {
		t = reflect.PointerTo(t)
	}
	return t
}

// tryInjectOwner 尝试对 controller/resolver 实例做依赖注入。
//
// 若实例所有 exported 字段都是零值，则认为用户没有手工赋值，走 Class provider 构造路径
// （ConstructorFinder 优先，否则 reflect.New + injectFields）；
// 若存在非零字段，保持原实例不变。
func (m *BaseModule) tryInjectOwner(owner any) any {
	if owner == nil {
		return nil
	}

	rv := reflect.ValueOf(owner)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return owner
	}

	for i := 0; i < rv.NumField(); i++ {
		if rv.Type().Field(i).IsExported() && !rv.Field(i).IsZero() {
			return owner
		}
	}

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

// Destroy 销毁模块（逆序处理 imports）。
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

// ModuleReexport 描述从另一个模块转发导出 token 的声明。
type ModuleReexport struct {
	Module Module
	Tokens []any
}

// Reexport 创建一个模块重导出声明。
func Reexport(module Module, tokens ...any) *ModuleReexport {
	return &ModuleReexport{Module: module, Tokens: tokens}
}

// exportedTokens 计算某个模块声明的 exports token 列表，并展开 ModuleReexport。
func exportedTokens(mod Module) []any {
	if mod == nil || mod.Metadata() == nil {
		return nil
	}
	out := make([]any, 0, len(mod.Metadata().Exports))
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

// ForwardRef 表示一个延迟求值的模块引用，用于打破声明阶段的循环引用。
type ForwardRef func() Module

// NewForwardRef 创建一个延迟求值的模块引用。
func NewForwardRef(fn func() Module) ForwardRef { return fn }
