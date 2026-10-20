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
func New(root module.Module, opts Options) (*Application, error)
```

`New` 是唯一构造入口，流程固定：

```text
加载模块树 → 各模块 Init → 发现并挂载适配器 → 合并模块贡献的全局 AOP
```

### Options

```go
type Options struct {
	Logger io.Writer   // 非空时输出启动报告
}
```

## 2. 装配

适配器不再作为参数传入，而是**以模块形式**出现在根模块的 `Imports` 里：

```go
root := module.New(module.Metadata{
	Imports: []module.Module{
		http.Module(http.Config{Addr: ":8080"}),
		swagger.Module(swagger.Config{Title: "API", Version: "1.0.0"}),
		users.Module(),
	},
	Controllers: []any{&UserController{}},
})
```

| 方法 | 说明 |
|------|------|
| `Adapter(id string) (adapter.Adapter, bool)` | 取已挂载的适配器 |
| `RegisterPlugin(plugins ...compiler.ProtocolPlugin)` | 追加自定义协议插件 |
| `Handle(pattern string, h http.Handler)` | 以宿主身份挂载原生 handler |
| `Static(prefix, root string)` | 挂载静态目录 |
| `MountHTTP(pattern string, h http.Handler) error` | `HTTPMountHost` 实现 |

## 3. 全局 AOP

```go
func (a *Application) UseGlobalMiddlewares(mws ...aop.Middleware)
func (a *Application) UseGlobalGuards(gs ...aop.Guard)
func (a *Application) UseGlobalInterceptors(is ...aop.Interceptor)
func (a *Application) UseGlobalPipes(ps ...aop.Pipe)
func (a *Application) UseGlobalFilters(fs ...aop.ExceptionFilter)
```

参数是接口类型，所以 `aop.GuardFunc` / `aop.GuardFactory(...)` 以及任意实现该接口的对象
都能直接传入；同签名的裸函数需要包一层（`aop.InterceptorFunc(fn)` 等）。

模块也可以在 `Init` 阶段贡献全局 AOP（等价 NestJS 的 `APP_*` 提供者）：

```go
m.Context().Use(module.AOP{Interceptors: []aop.Interceptor{aop.InterceptorFunc(myInterceptor)}})
```

两种来源在 `New` 阶段合并，顺序为：模块加载顺序 → `UseGlobal*` 调用顺序。

## 4. 启动

```go
func (a *Application) Listen(addr string) error  // 设置共享监听地址（HTTP 承载）
func (a *Application) Bootstrap() error          // 只执行 OnApplicationBootstrap（可重复调用）
func (a *Application) Run() error                // 声明 → 编译 → 启动适配器 → 阻塞
func (a *Application) Stop() error               // 逆序停止适配器 + shutdown 钩子
```

- `Listen` 只作用于 `RequiresSharedListen()` 为 true 的 adapter（当前只有 HTTP），
  且必须在 `Run` / `Start` 之前调用；WS / GraphQL 未配置 `Addr` 时会挂到它上面。
- `Bootstrap` 用于"不启动监听、只跑后台任务"的部署方式；`Run` 内部也会调用它，
  重复调用是幂等的。
- 如果 HTTP 模块已经写了 `Config{Addr}`，`Listen` 会覆盖它。

## 5. 诊断

| 方法 | 说明 |
|------|------|
| `Diagnostics() *compiler.CompileDiagnostics` | 最近一次编译诊断快照 |
| `LastCompileDiagnostics() *compiler.CompileDiagnostics` | 同上 |
| `StartupReport() (*StartupReport, error)` | 启动汇总（模块/路由/binding） |
| `GraphDiagnostics() *graph.Diagnostics` | 模块依赖图诊断 |
| `Runtime() *runtime.Runtime` | 运行时（Router/Executor） |
| `Registry() *registry.Registry` | 当前声明仓库 |
| `ModuleRef() *module.ModuleRef` | 模块引用树 |
| `HotLoad(mod module.Module) error` | 动态加载模块并重新声明/编译 |

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

`String()` 输出便于人读的文本。

## 6. 典型启动序列

```go
app, err := application.New(root, application.Options{Logger: os.Stdout})
if err != nil {
	log.Fatal(err)
}
if err := app.Run(); err != nil {
	log.Fatal(err)
}
```

声明（`Declare`）与编译都在 `Run` 内部完成，不需要手工调用
`CompileDiagnostics` / `StartupReport`。
