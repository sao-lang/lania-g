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
	Metadata() *Metadata
	Container() *di.Container
	Init() error
	Destroy() error
}
```

### Metadata

```go
type Metadata struct {
	Imports     []Module
	Providers   []*di.Provider
	Controllers []any
	Resolvers   []any
	Exports     []any
	IsGlobal    bool
	AOP         AOP // Middlewares / Guards / Interceptors / Pipes / Filters
}
```

## 2. 创建模块

```go
func New(metadata Metadata) *BaseModule
func AsGlobal(metadata Metadata) *GlobalModule
```

| 函数 | 用途 |
|------|------|
| `module.New` | 唯一的模块构造函数 |
| `module.AsGlobal` | 全局模块（provider 对所有模块可见） |

{{< callout type="info" >}}
不存在 `CreateModule` / `CreateAutoModule` / `SimpleModule` 等第二套构造函数。
自定义模块时，通过 `module.New` 创建实例并在 `Init()` 里使用 `m.Context()`；
需要暴露额外能力时实现 `module.Declarer`（`Declare(*Routes)`）即可。
{{< /callout >}}

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
func AsGlobal(metadata Metadata) *GlobalModule
```

等价于 `module.New(metadata)` 并置 `IsGlobal: true`，provider 对所有模块可见。

### 带配置的模块

集成包统一用 `X.Module(cfg)` 表达"带配置的模块"，返回值仍是 `module.Module`：

```go
httpadapter.Module(httpadapter.Config{Addr: ":8080", Validator: true})
auth.Module(auth.Config{Secret: "..."})
swagger.Module(swagger.Config{Title: "API"})
```

配置校验失败不会让 `Module()` 返回 `error`，而是生成一个"失败模块"，
在 `application.New` 阶段报出具体错误——保持装配代码无需逐层处理错误。

### 循环引用

```go
func NewForwardRef(fn func() Module) ForwardRef
```

用于 A ↔ B 互相 import 的场景。

### 模块作用域（Module Context）

```go
func (c *Context) Module() Module
func (c *Context) Key() string
func (c *Context) Container() *di.Container
func (c *Context) Registry() *registry.Registry
func (c *Context) Bind(name string, tokens ...any) error
func (c *Context) Use(aop module.AOP)
func (c *Context) AOP() module.AOP
func (c *Context) RegisterDecl(protocol string, kind string, items ...any)
```

`Init()` 里通过 `m.Context()` 拿到自己这一份上下文：

- `Bind(name, tokens...)` —— 把 provider 以命名 token 绑定到本模块容器（替代旧的 `SetRegistry`）
- `Use(aop)` —— 在模块级别追加中间件/守卫/拦截器，等价于 `Metadata.AOP`

{{< callout type="warning" >}}
模块身份按**实例**而非 `reflect.Type` 识别。`module.New` 对所有模块返回同一个
`*module.BaseModule` 具体类型，因此图校验与 `ModuleRef` 都用实例指针做 key。
{{< /callout >}}

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
