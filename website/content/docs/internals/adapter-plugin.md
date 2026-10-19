---
title: Adapter 扩展
weight: 2
prev: /docs/internals/request-flow
next: /docs/internals/diagnostics
---

新增一种协议 = 实现一个 Adapter + 一个 ProtocolPlugin。核心执行链无需改动。

## 1. Adapter 契约

```go
type Adapter interface {
	ID() string
	API() any
	Mount(host Host) error
	Start() error
	Stop() error
	Plugins() []compiler.ProtocolPlugin
}
```

可选能力：

| 接口 | 作用 |
|------|------|
| `SharedListenerConfigurator` | 与 HTTP 共用监听端口 |
| `HTTPMountHost`（host 侧） | 允许挂载原生 `http.Handler` |

## 2. Plugin 契约

```go
type ProtocolPlugin interface {
	ID() string
	Scan(moduleRef *module.ModuleRef, reg *registry.Registry) error
	Compile(...) ([]CompiledRoute, error)
}
```

典型 `Scan` 做三件事：

1. 从 registry 读取声明
2. 解析 receiver 的 owner 模块
3. 生成 `runtime.Handler` 并写入 registry

## 3. 最小实现骨架

```go
const AdapterID = "demo"

type Adapter struct {
	host adapter.Host
	api  *API
}

func (a *Adapter) ID() string   { return AdapterID }
func (a *Adapter) API() any     { return a.api }

func (a *Adapter) Mount(host adapter.Host) error {
	a.host = host
	a.api = NewAPI(host.Registry())
	return nil
}

func (a *Adapter) Plugins() []compiler.ProtocolPlugin {
	return []compiler.ProtocolPlugin{NewPlugin()}
}
```

## 4. 编译期要做的事

```go
func (p *Plugin) Scan(moduleRef *module.ModuleRef, reg *registry.Registry) error {
	for _, item := range reg.ListDecl(AdapterID, "routes") {
		def := item.(*RouteDefinition)
		owner := resolveOwner(moduleRef, def.Receiver) // 必须能找到模块
		handler, err := runtime.NewHandlerByToken(ownerToken, def.HandlerName)
		if err != nil {
			return err
		}
		handler.Meta.RouteKey = runtime.BuildRouteKey(AdapterID, def.Event, def.Namespace)
		reg.Register(hander...)
	}
	return nil
}
```

## 5. 运行期要做的事

1. 把传输层请求投影成 `runtime.HandlerContext`
2. 设置 `Request.Method/Path/RouteKey/Raw/Body`
3. 把协议特有数据写入 metadata（供 binding 读取）
4. 调用 `rt.Execute(ctx)`
5. 按协议编码结果

```go
rctx := runtime.AcquireHandlerContext("demo")
defer runtime.ReleaseHandlerContext(rctx)

rctx.Request.Method = event
rctx.Request.Path = namespace
rctx.RouteKey = runtime.BuildRouteKey("demo", event, namespace)
rctx.Request.Body = payload
rctx.Set("demo.socket", conn)

result, err := a.host.Runtime().Execute(rctx)
```

## 6. Binding 注册

```go
func RegisterDefaultsToRegistry(reg *registry.Registry) {
	reg.RegisterBindings(runtime.NewBindingResolvers(
		runtime.BindingRegistration{
			Name:             "Body",
			AllowedProtocols: map[runtime.Protocol]bool{"demo": true},
			Match:            matchBody,
			Resolve:          resolveBody,
		},
	)...)
}
```

{{< callout type="warning" >}}
`AllowedProtocols` 必须填写正确。多协议共存时，binding 解析会同时校验类型与协议；
协议不匹配的 resolver 会被跳过，而不是直接报错。
{{< /callout >}}

## 7. 自检清单

- [ ] `ID()` 与 plugin ID 一致
- [ ] `Mount` 中绑定 `host.Registry()`，不要用 `registry.Global()`
- [ ] 每个 route 都能解析到 owner 模块
- [ ] `ReleaseHandlerContext` 已 defer
- [ ] `Stop()` 释放 listener / server / goroutine
- [ ] 新增单测：`Compile` + `Execute` 两条路径
