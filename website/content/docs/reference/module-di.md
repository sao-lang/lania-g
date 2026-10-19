---
title: Module & DI
weight: 2
prev: /docs/reference/application
next: /docs/reference/registry-compiler-runtime
---

包：`kernel/v3/module`、`kernel/v3/di`

## 1. 模块接口

```go
type Module interface {
	Metadata() *ModuleMetadata
	Container() *di.Container
	Init() error
	Destroy() error
}
```

### ModuleMetadata

```go
type ModuleMetadata struct {
	Imports     []Module
	Controllers []any
	Resolvers   []any
	Providers   []*di.Provider
	Exports     []any
	IsGlobal    bool
}
```

## 2. 创建模块

```go
func CreateModule(imports []Module, providers []*di.Provider,
	controllers []any, resolvers []any, exports []any) Module

func CreateGlobalModule(...) Module
func CreateAutoModule(imports []Module, exports []any) (Module, error)
func SimpleModule() (Module, error)
func NewBaseModule(metadata *ModuleMetadata) *BaseModule
```

| 函数 | 用途 |
|------|------|
| `CreateModule` | 标准模块 |
| `CreateGlobalModule` | 全局模块（provider 对所有模块可见） |
| `CreateAutoModule` | 只声明 imports/exports，自动推导 |
| `SimpleModule` | 空模块 |
| `NewBaseModule` | 自定义模块时嵌入基础实现 |

## 3. 模块引用树

```go
type ModuleRef struct{}

func NewModuleRef(root Module) *ModuleRef
func (r *ModuleRef) Get(token any) (any, error)
func (r *ModuleRef) MustGet(token any) any
func (r *ModuleRef) GetContainer() *di.Container
func (r *ModuleRef) GetModule(moduleType any) (Module, error)
func (r *ModuleRef) MustGetModule(moduleType any) Module
func (r *ModuleRef) GetModuleContainerByType(t reflect.Type) (*di.Container, bool)
func (r *ModuleRef) GetAllModules() []Module
func (r *ModuleRef) GetAllModulesWithTypes() map[reflect.Type]Module
func (r *ModuleRef) GetRootModule() Module
```

泛型便捷：

```go
store, err := module.GetByType[*Store](app.ModuleRef())
store := module.MustGetByType[*Store](app.ModuleRef())
```

## 4. 模块加载器

```go
func NewModuleLoader(rootModules ...Module) *ModuleLoader
func (l *ModuleLoader) Load(root Module) (*LoadResult, error)
func (l *ModuleLoader) LoadMultiple(roots ...Module) (*LoadResult, error)
func (l *ModuleLoader) LoadModule(mod Module) error
func (l *ModuleLoader) GetModuleRef() *ModuleRef
func (l *ModuleLoader) GetLoadedModules() []Module
func (l *ModuleLoader) GetDiagnostics() *graph.Diagnostics
func (l *ModuleLoader) IsLoaded(moduleType any) bool
```

加载顺序：深度优先递归 `imports` → 逐模块 `Init()`。

## 5. 高级模块

### 全局模块

```go
func NewGlobalModule(metadata *ModuleMetadata) *GlobalModule
```

### 动态模块

```go
type DynamicModule interface {
	ForRoot(options any) (Module, error)
	ForRootAsync(options AsyncOptions) (Module, error)
	ForFeature(options any) (Module, error)
	Register(options any) (Module, error)
	RegisterAsync(options AsyncOptions) (Module, error)
}

func NewDynamicModule(factory any) *DynamicModuleBase
```

### 循环引用

```go
func NewForwardRef(fn func() Module) ForwardRef
```

用于 A ↔ B 互相 import 的场景。

### 装饰器风格

```go
func ModuleDecorator(metadata *ModuleMetadata) func(any) Module
func Global() func(any) Module
```

## 6. DI 容器

```go
func NewContainer() *Container
func (c *Container) NewChild() *Container
func (c *Container) SetParent(parent *Container)
func (c *Container) Register(provider *Provider)
func (c *Container) RegisterValue(token any, value any)
func (c *Container) RegisterFactory(token any, factory func() (any, error), scope Scope)
func (c *Container) RegisterClass(token any, typ reflect.Type, scope Scope)
func (c *Container) RegisterExisting(token any, existing any)
func (c *Container) Get(token any) (any, error)
func (c *Container) GetAll() map[any]*Provider
func (c *Container) SetConstructorFinder(finder ConstructorFinder)
```

泛型辅助：

```go
func RegisterValue[T any](c *Container, token any, value T)
func RegisterFactory[T any](c *Container, token any, factory func() (T, error), scope Scope)
func RegisterClass[T any](c *Container, token any, scope Scope)
func Get[T any](c *Container, token any) (T, error)
func GetByType[T any](c *Container) (T, error)
func MustGet[T any](c *Container, token any) T
func MustGetByType[T any](c *Container) T
```

## 7. Provider

```go
type Provider struct {
	Token       any
	Type        ProviderType // Value / Factory / Class / Existing
	UseValue    any
	UseFactory  func() (any, error)
	UseClass    reflect.Type
	UseExisting any
	Scope       Scope
}

func ProviderFromInstance[T any](instance T, scope Scope) (*Provider, error)
func ProviderFromInstanceWithToken[T any](token any, instance T, scope Scope) (*Provider, error)
func ProviderFromFactory[T any](token any, factory func() (T, error), scope Scope) (*Provider, error)
```

## 8. 作用域

| Scope | 缓存位置 |
|-------|----------|
| `di.Singleton` | provider 所在容器 |
| `di.Request` | 请求级 child container |
| `di.Transient` | 不缓存 |

## 9. 注入规则

1. **构造函数注入**：`ConstructorFinder` 命中时优先
2. **`Injectable` 接口**：`Inject(c *Container) error`
3. **字段注入**：exported、零值、容器中存在类型
4. **handler 参数**：binding 未命中时回退 DI

## 10. 解析与缓存

- token 统一使用 `reflect.Type`
- 循环依赖检测基于解析栈，返回 `ErrCircularDependency`
- 字段注入对 nil / 不可寻址 / 类型不匹配的依赖静默跳过
