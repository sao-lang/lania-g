---
title: 集成
weight: 4
sidebar:
  open: true
toc: false
---

`integrations/*/v3` 是可插拔的基础设施模块，全部遵循同一套约定：

```go
mod, err := logger.Module(logger.Config{Level: "info"})
```

返回的 `module.Module` 直接放进根模块的 `imports` 即可。

## 已提供的能力

| 集成 | 作用 | 依赖 |
|------|------|------|
| `auth` | 认证鉴权 | 无 |
| `cache` | 内存 / Redis 缓存 | 可选 Redis |
| `config` | 配置加载 | 无 |
| `logger` | 结构化日志 | 无 |
| `orm` | ORM + 事务 | 数据库 |
| `mongodb` | MongoDB + Repository | MongoDB |
| `otel` | OpenTelemetry | Collector |
| `outbox` | Outbox 发件箱 | Store |
| `resilience` | 熔断 / 限流 / 幂等 | 无 |
| `events` | 事件总线 | 无 |
| `http` | HTTP 客户端 | 无 |
| `grpc` | gRPC 客户端 | 无 |
| `swagger` | OpenAPI 文档 | 无 |
| `terminus` | 健康检查 | 无 |
| `kafka` | Kafka 客户端 | Kafka |
| `minio` | 对象存储 | MinIO |
| `es` | Elasticsearch | ES |

## 装配示例

```go
root := module.New(module.Metadata{
	Imports: []module.Module{
		logger.Module(logger.Config{Level: logger.InfoLevel}),
		cache.Module(cache.Config{Type: cache.Memory}),
		swagger.Module(swagger.Config{Title: "Demo API", Version: "1.0.0"}),
	},
	Controllers: []any{ctrl},
})
```

## 约定

1. 所有集成通过 `X.Module(cfg)` 返回 `module.Module`，直接放进根模块的 `Imports`
2. 导出的 token 统一使用 `reflect.TypeFor[T]()`
3. 需要外部服务的集成在无环境时应显式标注"未验证（需环境）"
4. 集成内部不直接依赖具体协议，只依赖 `kernel` 抽象
5. 需要注入服务时从请求容器取（`serviceOf(ctx)`），不在构造期传入

详细接入步骤见仓库 `docs/` 下的专项文档（auth / mongodb / otel / outbox / resilience）。
