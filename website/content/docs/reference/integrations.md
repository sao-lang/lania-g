---
title: 集成
weight: 12
prev: /docs/reference/scheduler
next: /docs/reference/errors
---

`integrations/*/v3` 是可插拔的基础设施模块，统一约定：

```go
mod, err := logger.Module(logger.Config{Level: logger.InfoLevel})
```

返回的 `module.Module` 放进根模块的 `imports`；框架把它们的导出 token 自动注册进容器。

## 1. 全景

| 集成 | 包路径 | 关键配置字段 |
|------|--------|--------------|
| `auth` | `integrations/auth/v3` | `JWTSecret` / `JWTIssuer` / `JWTAudience` / `APIKeys` / `Sessions` / `IdentityProviders` / `ClaimsValidators` |
| `cache` | `integrations/cache/v3` | `Type`（`cache.Memory` / `cache.Redis`）/ `Name` / `Redis` |
| `config` | `integrations/config/v3` | `Files` / `EnvPrefix` / `Data` |
| `es` | `integrations/es/v3` | `Name` / `Addresses` / `Username` / `Password` |
| `events` | `integrations/events/v3` | `Async` |
| `grpc` | `integrations/grpc/v3` | `Name` / `Target` / `DialTimeout` / `Insecure` / `DialOptions` / `CallOptions` |
| `http` | `integrations/http/v3` | `Name` / `BaseURL` / `Timeout` / `RetryCount` / `RetryDelay` / `RetryDelayMultiplier` / `DefaultHeaders` / `ProxyURL` / 三类 Interceptor |
| `kafka` | `integrations/kafka/v3` | `Name` / `Brokers` / `Dialer` / `Writer*` 系列 |
| `logger` | `integrations/logger/v3` | `Name` / `Level` / `Driver` / `Format` / `Output` / `Hooks` / `ContextKeys` |
| `minio` | `integrations/minio/v3` | `Name` / `Endpoint` / `AccessKeyID` / `SecretAccessKey` / `UseSSL` |
| `mongodb` | `integrations/mongodb/v3` | `Name` / `URI` / `Database` / `Client` / `Options` |
| `orm` | `integrations/orm/v3` | `Name` / `DB` / `Dialector` / `GormConfig` / `AutoMigrate` / `Plugins` / `Managed` |
| `otel` | `integrations/otel/v3` | `ServiceName` / `InstrumentationName` / `Propagator` / `Exporters` / `Metrics` / `Resource` |
| `outbox` | `integrations/outbox/v3` | `Name` / `MaxAttempts` / `Store` / `Dispatcher` / `DeadLetter` / `Scheduler` |
| `resilience` | `integrations/resilience/v3` | `RateLimit` / `Timeout` / `Retry` / `Circuit` / `Idempotency` / `DistributedStore` / `Strategy` |
| `swagger` | `integrations/swagger/v3` | `Title` / `Description` / `Version`（+ `UIConfig`） |
| `terminus` | `integrations/terminus/v3` | `Version` / `ReleaseID` |

## 2. 装配示例

```go
loggerMod, _ := logger.Module(logger.Config{Level: logger.InfoLevel})
cacheMod, _ := cache.Module(cache.Config{Type: cache.Memory})
eventsMod, _ := events.Module(events.Config{Async: true})
swaggerMod, _ := swagger.Module(
	swagger.Config{Title: "Demo API", Version: "1.0.0"},
	&swagger.UIConfig{Title: "Docs", SwaggerURL: "/docs", SpecURL: "/docs/openapi.json"},
)

root := module.CreateModule(
	[]module.Module{loggerMod, cacheMod, eventsMod, swaggerMod},
	providers,
	[]any{ctrl},
	nil, nil,
)
```

## 3. 重点集成

### auth

```go
authMod, _ := auth.Module(auth.Config{
	JWTSecret: "change-me",
	JWTIssuer: "lania",
	APIKeys:   map[string]auth.Principal{"key-1": {Subject: "svc"}},
	Sessions:  map[string]auth.Principal{"sess-1": {Subject: "user-1"}},
})
```

提供 `Principal` 注入：`AuthUser` / `AuthUserID` / `AuthRole` / `AuthPermission` 等 HTTP wrapper。

### orm

```go
ormMod, _ := orm.Module(orm.Config{
	Dialector: sqlite.Open("file::memory:?cache=shared"),
	GormConfig: &gorm.Config{},
	AutoMigrate: []any{&User{}},
	Managed: true,     // 框架负责关闭连接
})
```

多数据源用 `orm.MultiConfig{DefaultName, DataSources}`。

### outbox

```go
outboxMod, _ := outbox.Module(outbox.Config{
	MaxAttempts: 5,
	Store:       outbox.NewGormStore(db),
	Dispatcher:  kafkaDispatcher,
	DeadLetter:  deadLetterDispatcher,
	Scheduler:   outbox.SchedulerConfig{Interval: time.Second, BatchSize: 100},
})
```

语义：业务事务内落一条 `outbox.Message`，后台轮询可靠投递，失败重试至 `MaxAttempts` 后进死信。

### resilience

```go
resMod, _ := resilience.Module(resilience.Config{
	RateLimit: resilience.RateLimitConfig{Enabled: true, Limit: 100, Period: time.Second},
	Timeout:   resilience.TimeoutConfig{Enabled: true, Duration: 3 * time.Second},
	Retry:     resilience.RetryConfig{Enabled: true, Attempts: 3, Backoff: 100 * time.Millisecond},
	Circuit:   resilience.CircuitBreakerConfig{Enabled: true, Threshold: 5, ResetTimeout: 30 * time.Second},
})
```

### terminus

```go
termMod, _ := terminus.Module(terminus.Config{Version: "1.0.0", ReleaseID: "dev"})
```

配合 `terminus` 的 health service 在 `/health` 暴露就绪/存活探针。

### otel

```go
otelMod, _ := otel.Module(otel.Config{
	ServiceName: "demo",
	Exporters: []otel.ExporterConfig{
		{Type: "otlp", Endpoint: "127.0.0.1:4317"},
	},
})
```

自动注入 trace/parent 传播与请求计数指标。

## 4. 约定

1. 所有集成通过 `ForRoot(cfg)` 返回 `module.Module`
2. 导出 token 统一用 `reflect.TypeFor[T]()` 表达
3. 集成只依赖 `kernel` 抽象，不依赖具体协议
4. 需要外部服务的集成在没有环境时（Kafka / MongoDB / ES / MinIO / OTel）应显式标注「未验证（需环境）」
5. 每个集成都提供 `Factory` 接口（`Default()` / `New(cfg)`），可注入测试替身

## 5. 常见用法

```go
// 从模块里取导出实例
loggerFactory, err := module.GetByType[*logger.Factory](app.ModuleRef())
log := loggerFactory.Default()
log.Info("boot", logger.String("addr", ":8080"))

// 缓存
cacheFactory, _ := module.GetByType[*cache.Factory](app.ModuleRef())
c := cacheFactory.Default()
_ = c.Set(ctx, "k", []byte("v"), time.Minute)

// HTTP 客户端
httpFactory, _ := module.GetByType[*httpclient.Factory](app.ModuleRef())
client := httpFactory.Default()
resp, err := client.Get(ctx, "/users")
```
