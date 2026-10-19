---
title: 测试与部署
weight: 3
prev: /docs/guides/multi-protocol
---

## 1. 构建

```bash
# 单个服务
cd cmd/http-demo
go build -o bin/http-demo .

# 全模块验证
$mods = Get-ChildItem -Path application,cmd,integrations,kernel,protocol -Recurse -Filter go.mod |
  ForEach-Object { $_.Directory.FullName }
foreach ($m in $mods) { Push-Location $m; go build ./...; go test ./...; Pop-Location }
```

## 2. 端口与配置

示例统一支持：

| 变量 | 作用 |
|------|------|
| `PORT` | 通用端口（自动补 `:`） |
| `APP_ADDR` | HTTP/WS/GraphQL 监听地址 |
| `GRPC_ADDR` | gRPC 监听地址 |

生产环境建议从环境变量或 `integrations/config` 读取，而不是硬编码。

## 3. 健康检查

```go
root := module.New(module.Metadata{
	Imports: []module.Module{
		terminus.Module(terminus.Config{Path: "/health"}),
	},
	Controllers: []any{ctrl},
})
```

按集成配置暴露 `/health`（或自定义路径），供负载均衡与 K8s 探针使用。

## 4. 优雅关闭

```go
if err := app.Run(); err != nil { // 阻塞，直到收到退出信号
	log.Fatal(err)
}
```

```go
sig := make(chan os.Signal, 1)
signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

go func() {
	<-sig
	if err := app.Stop(); err != nil {
		log.Println("stop:", err)
	}
}()
```

`Stop()` 会按 adapter 挂载逆序停止，再执行模块/应用 shutdown 钩子。

## 5. 可观测性

| 能力 | 接入 |
|------|------|
| 结构化日志 | `integrations/logger` |
| 链路追踪 / 指标 | `integrations/otel` |
| 健康检查 | `integrations/terminus` |
| 调度快照 | `scheduler.MountHTTPBridge(app, adp, "/scheduler/status")` |
| 启动诊断 | `app.StartupReport()` / `app.GraphDiagnostics()` |

## 6. 容器化要点

```dockerfile
FROM golang:1.25 AS build
WORKDIR /src
COPY . .
RUN cd cmd/http-demo && CGO_ENABLED=0 go build -o /out/app .

FROM gcr.io/distroless/static
COPY --from=build /out/app /app
ENV PORT=:8080
EXPOSE 8080
ENTRYPOINT ["/app"]
```

注意：

- 多模块工作区需要在**仓库根目录**复制源码后进入子模块构建
- 静态编译可避免 glibc 依赖
- 探针建议走 `terminus`，不要直接探业务路由

## 7. 上线前检查清单

```text
- [ ] 启动日志中 StartupReport 无 warning
- [ ] StartupReport 中 routeCount 数量符合预期
- [ ] 没有依赖已移除的全局 registry / *Compat 入口
- [ ] 所有外部集成都有超时与重试
- [ ] 健康检查与优雅关闭已验证
- [ ] 关键路径有端到端测试
```
