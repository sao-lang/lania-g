# lania-g v3 OTel 接入

## 1. 目标

`integrations/otel` 提供第三阶段可观测性能力的第一批落地：

- tracing
- metrics
- context propagation
- trace id / request id 贯通
- logger 字段桥接

当前实现优先解决框架内统一接入问题，不强绑具体 exporter。

## 2. 基本接入

```go
package main

import (
	"os"

	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
	"github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	loggerintegration "github.com/sao-lang/lania-g/integrations/logger/v3"
	otelintegration "github.com/sao-lang/lania-g/integrations/otel/v3"
)

type UserController struct{}

func (c *UserController) Ping() map[string]string {
	return map[string]string{"ok": "true"}
}

func main() {
	loggerModule, err := loggerintegration.ForRoot(loggerintegration.Config{
	ContextKeys: otelintegration.LoggerContextKeys(),
	})
	if err != nil {
		panic(err)
	}

	otelModuleRaw, err := otelintegration.ForRoot(otelintegration.Config{
		ServiceName: "demo-service",
	})
	if err != nil {
		panic(err)
	}
	otelModule := otelModuleRaw.(*otelintegration.Module)

	ctrl := &UserController{}
	root := module.CreateModule(
		[]module.Module{loggerModule, otelModule},
		nil,
		[]interface{}{ctrl},
		nil,
		nil,
	)

	httpAdapter := httpadapter.New()
	app, err := application.NewWithOptions(root, application.Options{
		Registry:        application.NewRegistry(),
		StartupReporter: os.Stdout,
	}, httpAdapter)
	if err != nil {
		panic(err)
	}

otelintegration.Install(app, otelModule.Telemetry())

	httpAPI := httpAdapter.API().(*httpadapter.API)
	if _, err := httpAPI.Controller("/users", ctrl).
		Get("/ping", ctrl.Ping).
		BuildE(); err != nil {
		panic(err)
	}

	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}
}
```

## 3. 请求期行为

- middleware
  - 从请求头提取 `traceparent`
  - 生成或继承 `request id`
  - 把 `trace id` / `span id` / `request id` 写入 `HandlerContext`
  - 回写 `traceparent` / `request id` 到响应头
- interceptor
  - 为当前 route 启动 span
  - 记录基础 attributes
  - 统计请求计数
  - 在错误时记录 span error/status

## 4. 安装方式

推荐直接使用安装 helper：

```go
otelintegration.Install(app, telemetry)
```

如果使用 `factory.NestFactory`，可使用：

```go
if err := otelintegration.InstallOnFactory(factory, telemetry); err != nil {
	panic(err)
}
```

## 5. Logger 桥接

可以直接从 `HandlerContext` 提取 logger 字段：

```go
fields := otelintegration.LoggerFields(handlerCtx)
lg = lg.With(fields...)
```

也可以依赖 logger 的 `ContextKeys`，因为 `integrations/otel` 会把以下 key 放入 `context.Context`：

- `otel.trace_id`
- `otel.span_id`
- `otel.request_id`

## 6. Handler 参数注入

当前支持以下绑定包装器：

- `otelintegration.InjectTelemetry`
- `otelintegration.TelemetryRef[T]`
- `otelintegration.InjectTraceContext`
- `otelintegration.InjectRequestID`

## 7. 高级特性

### 7.1 真实 Exporter 支持

现在支持多种真实的exporter：

```go
otelModuleRaw, err := otelintegration.ForRoot(otelintegration.Config{
    ServiceName: "demo-service",
    Exporters: []otelintegration.ExporterConfig{
        {
            Type:       "jaeger",
            Endpoint:   "http://localhost:14268/api/traces",
            Sampler:    0.1,
            BatchSize:  1000,
            Interval:   5 * time.Second,
        },
        {
            Type: "stdout",
        },
    },
})
```

支持的exporter类型：
- `jaeger`：Jaeger 分布式追踪系统
- `stdout`：控制台输出
- `prometheus`：Prometheus 指标监控

### 7.2 更多指标

现在收集更多指标：

- 请求计数：`lania.request.count`
- 请求持续时间：`lania.request.duration`
- 错误计数：`lania.error.count`
- 吞吐量：`lania.throughput`

配置示例：

```go
otelModuleRaw, err := otelintegration.ForRoot(otelintegration.Config{
    ServiceName: "demo-service",
    Metrics: otelintegration.MetricConfig{
        RequestDurationName: "lania.request.duration",
        ErrorCounterName:    "lania.error.count",
        ThroughputName:      "lania.throughput",
        Labels:              map[string]string{"env": "production"},
    },
})
```

### 7.3 协议级更细桥接

自动为不同协议添加协议特定的属性和标签：

- HTTP：method、path、status code
- gRPC：service、method、status code
- WebSocket：action、connection id

### 7.4 后续扩展

- OTLP exporter
- 更细粒度业务指标
- 日志与 trace 的更深层关联
