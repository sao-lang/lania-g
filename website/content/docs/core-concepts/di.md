---
title: 依赖注入
weight: 2
prev: /docs/core-concepts/module
next: /docs/core-concepts/compile-runtime
---

`lania-g` 的容器是一个**父子容器树**：每个模块拥有自己的容器，子模块从父模块继承可见的 provider。

## 1. 三种注册方式

```go
// 1) 值 / 实例（推荐：免 token）
p, _ := di.ProviderFromValue[*Store](store, di.Singleton)

// 需要自定义 token（例如按接口注册）时用带 token 的版本
p, _ := di.ProviderFromInstanceWithToken(di.TokenFor[StoreContract](), store, di.Singleton)

// 2) 构造函数
p := di.ProviderFromFactory(di.TokenFor[*Service](), func() (*Service, error) {
	return NewService(), nil
}, di.Singleton)

// 3) 类型（由容器反射构造实例）
p, _ := di.ProviderFromInstance[*Service](&Service{}, di.Transient)
```

{{< callout type="info" >}}
`di.TokenFor[T]()` 等价于 `reflect.TypeFor[T]()`，业务代码因此不必 import `reflect`。
`ProviderFromValue[T]` 与 `ProviderFromInstanceWithToken(TokenFor[T](), ...)` 完全等价，
区别只是省掉了 token 参数。
{{< /callout >}}

## 2. 作用域

| 作用域 | 实例缓存位置 | 适用场景 |
|--------|--------------|----------|
| `di.Singleton` | provider 所在容器 | 无状态服务、连接池 |
| `di.Request` | 每次请求的 child container | 请求级上下文、事务 |
| `di.Transient` | 不缓存 | 轻量对象、必须独立实例 |

## 3. token 约定

框架内部统一用 `reflect.Type` 作为 token。业务代码不需要 import `reflect`：

```go
token := di.TokenFor[*MemoryStore]()   // 等价于 reflect.TypeFor[*MemoryStore]()
```

完整例子（注册 + 导出）：

```go
func StoreModule(store *Store) module.Module {
	provider, _ := di.ProviderFromValue[*Store](store, di.Singleton)
	return module.New(module.Metadata{
		Providers: []*di.Provider{provider},
		Exports:   []any{di.TokenFor[*Store]()},
	})
}
```

泛型便捷方法：

```go
store, err := di.GetByType[*MemoryStore](c)
store := di.MustGetByType[*MemoryStore](c)
```

## 4. 注入方式

### 4.1 字段注入（最常用）

Controller 以**零值**传入模块，`Init` 阶段自动填充 exported 且为零值的字段：

```go
type UserController struct {
	Store *MemoryStore // 自动注入
	Cache *Cache       // 自动注入
}
```

规则：

- 只注入 exported 字段
- 只注入零值字段（已有值不覆盖）
- 容器中不存在的类型**静默跳过**
- 支持沿 parent 链回溯

### 4.2 构造函数注入

注册一个构造函数，参数由容器解析：

```go
func NewUserService(store *MemoryStore, log *slog.Logger) *UserService {
	return &UserService{store: store, log: log}
}
```

### 4.3 `Injectable` 接口

适合不想暴露构造函数、但需要容器参与初始化的类型：

```go
type Injectable interface {
	Inject(c *di.Container) error
}
```

### 4.4 handler 参数注入

handler 参数既可以是协议 binding wrapper，也可以是容器里的任意类型：

```go
func (c *UserController) List(
	args listUsersArgs,          // 协议 binding
	store *MemoryStore,          // DI 兜底
	logger *slog.Logger,         // DI 兜底
) (any, error) { ... }
```

解析顺序：**协议 binding 命中优先 → 否则回退到 DI**。

## 5. 循环依赖

容器会记录解析栈，检测到环时返回 `ErrCircularDependency`：

```text
A -> B -> A  ✗
```

解决方式：

- 提取共同依赖到第三个 provider
- 用 setter / 延迟解析打破环
- 如果是请求级依赖，考虑改成 `di.Request` 作用域

## 6. 容器树示意

```text
Root Container
├── StoreModule Container      (Store provider)
├── CacheModule Container      (Cache provider)
└── Root (controllers)         (可见 imports 的 exports)
```

请求期会为每个请求创建 child container，`di.Request` 作用域的实例缓存在这里。
