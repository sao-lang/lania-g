---
title: 常见问题排查
weight: 2
prev: /docs/best-practices/testing
---

按错误信息查根因。

## 1. 启动期

| 错误 | 根因 | 解决 |
|------|------|------|
| `directory prefix . does not contain modules` | 在仓库根目录执行 `go build ./...` | 进入子模块目录 |
| `application.New requires a root module` | 传了 nil | 用 `module.New(module.Metadata{...})` 构造根模块 |
| `bind: Only one usage of each socket address` | 端口被占用 | `PORT` / `APP_ADDR` 覆盖 |
| `cannot infer grpc request type` | 方法没有请求字段（旧版本） | v3 已默认 `Empty`；旧版本请显式 `WithReqType` |
| `duplicate ws route for namespace=... event=...` | 同一 namespace+event 声明两次 | 删除重复声明 |
| `grpc stream method ... should return only error` | 流式方法返回了非 error 值 | 调整签名 |

## 2. 运行期

| 错误 | 根因 | 解决 |
|------|------|------|
| `provider not found` | provider 未注册或未导出 | 检查 `Providers` 与 `Exports` |
| `circular dependency` | A↔B 互相依赖 | 提取第三方依赖或延迟解析 |
| `binding not found` | 参数类型没有 resolver | 用 wrapper，或注册 provider |
| `binding not supported for protocol` | 类型命中了其他协议的 binding | 检查 wrapper 与 `AllowedProtocols` |
| `validation failed on X` | DTO 校验失败 | 检查 `validate` tag / 请求体 |
| `graphql field X.Y not found` | 查询字段不存在 | 对照 schema |
| `graphql type X not found` | 返回类型未绑定模型 / 嵌套类型未注册 | 确认 `.Returns(...)` 与 Go 类型一致 |
| `json: cannot unmarshal object into Go value of type string` | `Header[string]` 没有绑定名 | 用 `header:"X"` 或读 `HandlerContext` |
| `proto: invalid type: main.xxx` | 把 Go struct 塞进 `structpb.NewStruct` | 用 `grpcvalue.ToStruct` |
| WS 连接 400/EOF | 客户端与服务端 Engine.IO 版本不一致 | 服务端为 v4，客户端用 `socket.io-client@4` |

## 3. 路由不生效

按顺序检查：

```text
1. `app.Diagnostics()` 中 routes 数量是否为 0
2. `application.New` 是否返回编译/声明错误（冲突会整体失效）
3. receiver 是否放进模块的 controllers / resolvers 槽位
4. 是否使用了 registry.Global() 造成声明分裂
5. basePath / mount prefix 是否与请求路径匹配
```

## 4. 多协议同进程

| 现象 | 原因 | 解决 |
|------|------|------|
| 只能有一个 shared listener | `Listen` 检测到多个共享监听 adapter | 只保留一个 HTTP 类 adapter 用 `New()` |
| GraphQL 字段解析到 HTTP binding | 旧版本 binding 按类型抢占 | v3 已按 `(type, protocol)` 匹配 |
| WS 与 HTTP 端口冲突 | 两个协议模块配置了相同地址 | 分别配置 `ws.Config{Addr}` 与 `http.Config{Addr}` |

## 5. 日志与观测

- 设置 `application.Options{Logger: os.Stdout}` 观察启动报告
- 用 `/scheduler/status` 查看调度快照
- 用 `terminus` 暴露健康检查
- 需要链路追踪时接 `otel`

## 6. 提交前自检

```bash
go vet ./...
go test ./...
```

```text
- [ ] `application.New` 无错误
- [ ] `app.Diagnostics()` 中 routes 数量符合预期
- [ ] 没有使用 registry.Global()
- [ ] 端到端请求已手工验证
```
