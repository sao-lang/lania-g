---
title: Application
weight: 1
prev: /docs/reference
next: /docs/reference/module-di
---

包：`github.com/sao-lang/lania-g/application/v3`

`Application` 是装配门面：加载模块树、挂载 adapter、编译声明、安装到 Runtime、启动与关闭。

## 1. 创建

```go
func New(root module.Module, adapters ...adapter.Adapter) (*Application, error)
func NewCompat(root module.Module, adapters ...adapter.Adapter) (*Application, error)
func NewWithOptions(root module.Module, opts Options, adapters ...adapter.Adapter) (*Application, error)
```

| 入口 | Registry | 用途 |
|------|----------|------|
| `NewWithOptions` | 必须显式传入 | **推荐** |
| `New` | `registry.Global()` | 兼容旧代码 |
| `NewCompat` | `registry.Global()`，显式标记兼容 | 迁移期 |

### Options

```go
type Options struct {
	Registry        *Registry   // 实例级 registry（必填）
	StartupReporter io.Writer   // 启动报告输出目标（可选）
}
```

`NewRegistry()` 是 `registry.New()` 的别名。

## 2. 装配

| 方法 | 说明 |
|------|------|
| `UseAdapter(adp adapter.Adapter) error` | 挂载 adapter（会调用 `Mount` 并收集 Plugin） |
| `RegisterPlugin(plugins ...compiler.ProtocolPlugin)` | 注册自定义协议插件 |
| `API(adapterID string) (any, bool)` | 取已挂载 adapter 的 DSL API |
| `SetGlobalPrefix(prefix string)` | 为支持前缀的 adapter 设置全局前缀 |
| `MountHTTP(pattern string, handler http.Handler) error` | 以宿主身份挂载原生 handler（实现 `HTTPMountHost`） |

## 3. 全局 AOP

```go
func (a *Application) UseGlobalMiddlewares(mws ...aop.MiddlewareFunc)
func (a *Application) UseGlobalGuards(gs ...aop.GuardFunc)
func (a *Application) UseGlobalInterceptors(is ...aop.InterceptorFunc)
func (a *Application) UseGlobalPipes(ps ...aop.PipeFunc)
func (a *Application) UseGlobalFilters(fs ...aop.ExceptionFilterFunc)
```

语义与执行顺序见 [AOP](aop)。

## 4. 启动

```go
func (a *Application) Start() error   // = Run()
func (a *Application) Run() error     // 使用各 adapter 自己的地址
func (a *Application) Listen(addr string) error // 为共享监听 adapter 注入地址
func (a *Application) WaitStop()      // 阻塞直到 Stop
func (a *Application) Stop() error    // 逆序停止 adapter + shutdown 钩子
```

- `Run()`：所有 adapter 都必须能自行监听；存在共享监听 adapter 时报错
- `Listen(addr)`：把一个共享监听地址分发给唯一需要它的 adapter
- `Run/Listen` 会阻塞到 `Stop()`，符合长驻服务语义

## 5. 诊断

| 方法 | 说明 |
|------|------|
| `CompileDiagnostics() (*compiler.CompileDiagnostics, error)` | 触发（或复用）编译并返回诊断 |
| `LastCompileDiagnostics() *compiler.CompileDiagnostics` | 最近一次编译诊断快照 |
| `StartupReport() (*StartupReport, error)` | 启动汇总（模块/路由/binding/回退） |
| `GraphDiagnostics() *graph.Diagnostics` | 模块依赖图诊断 |
| `Runtime() *runtime.Runtime` | 运行时（Router/Executor） |
| `Registry() *Registry` | 当前 registry |
| `ModuleRef() *module.ModuleRef` | 模块引用树 |
| `HotLoad(mod module.Module) error` | 动态加载模块 |

### CompileDiagnostics 字段

```go
RegisteredPlugins        []string
ProtocolOrder            []runtime.Protocol
DeclarationCounts        map[string]map[string]int
BindingResolverCount     int
RegistrySource           string
GlobalAOP                AOPDiagnostics
Protocols                map[runtime.Protocol]*ProtocolDiagnostics
RegistryFallbacks        []RegistryFallbackDiagnostics
RuntimeFallbacks         []RuntimeFallbackDiagnostics
CompatFallbackCategories []CompatFallbackCategorySummary
CompatFallbackSources    []CompatFallbackSourceSummary
RouteConflicts           []RouteConflict
Errors                   []string
Warnings                 []string
```

JSON 键名固定为 lowerCamelCase，只增不删。

### StartupReport 字段

```go
ModuleCount           int
AdapterCount          int
ProtocolCount         int
RouteCount            int
BindingResolverCount  int
RegistrySource        string
GlobalAOP             AOPDiagnostics
Adapters              []AdapterStartupReport
Protocols             []ProtocolStartupReport
RegistryFallbacks     []RegistryFallbackDiagnostics
RuntimeFallbacks      []RuntimeFallbackDiagnostics
Integrations          []string
Modules               []ModuleStartupReport
```

`String()` 输出便于人读的文本；`HasCompatFallbacks()` 判断是否走了全局回退。

## 6. 典型启动序列

```go
app, _ := application.NewWithOptions(root, application.Options{
	Registry:        application.NewRegistry(),
	StartupReporter: os.Stdout,
}, httpAdapter)

// 声明...
if _, err := app.CompileDiagnostics(); err != nil { panic(err) }
if _, err := app.StartupReport(); err != nil { panic(err) }
if err := app.Listen(":8080"); err != nil { panic(err) }
```

## 7. 兼容语义

- `New` / `NewCompat` 使用 `registry.Global()`，适用于迁移期
- 所有协议 integration 都提供对应的 `*Compat` 入口
- 启动报告会列出 `registryFallbacks` / `runtimeFallbacks`，便于识别仍在走回退的代码
