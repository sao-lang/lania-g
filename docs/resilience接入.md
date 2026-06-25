# lania-g v3 Resilience 接入

## 1. 提供能力

`integrations/resilience` 当前提供：

- 限流
- 超时
- 重试
- 熔断
- 幂等

## 2. 基本接入

```go
resilienceModule, err := resilience.ForRoot(resilience.Config{})
if err != nil {
	panic(err)
}
resilienceValue := resilienceModule.(*resilience.Module)
resilience.Install(app, resilienceValue.Service())
```

## 3. 接入方式

- `Middleware`
  - 限流
- `Interceptor`
  - 超时、重试、熔断、幂等结果重放

## 4. 设计说明

- 不依赖 HTTP 专属语义
- 优先复用 `HandlerContext` 的 header / metadata
- 适合在 HTTP / gRPC / WS / GraphQL 共享同一套治理策略

## 5. 高级特性

### 5.1 分布式限流

现在支持分布式限流，通过分布式存储实现：

```go
// 实现 DistributedStore 接口
type RedisStore struct {
    client *redis.Client
}

func (r *RedisStore) Get(key string) (string, error) {
    return r.client.Get(key).Result()
}

func (r *RedisStore) Set(key string, value string, ttl time.Duration) error {
    return r.client.Set(key, value, ttl).Err()
}

func (r *RedisStore) Delete(key string) error {
    return r.client.Del(key).Err()
}

func (r *RedisStore) Exists(key string) (bool, error) {
    result, err := r.client.Exists(key).Result()
    return result > 0, err
}

// 配置分布式限流
resilienceModule, err := resilience.ForRoot(resilience.Config{
    DistributedStore: &RedisStore{client: redisClient},
    RateLimit: resilience.RateLimitConfig{
        Enabled: true,
        Limit:   100,
        Window:  time.Minute,
    },
})
```

### 5.2 持久化熔断状态

现在支持熔断状态的持久化，通过分布式存储实现：

```go
resilienceModule, err := resilience.ForRoot(resilience.Config{
    DistributedStore: &RedisStore{client: redisClient},
    Circuit: resilience.CircuitBreakerConfig{
        Enabled:          true,
        FailureThreshold: 5,
        OpenTimeout:      30 * time.Second,
    },
})
```

### 5.3 更细粒度策略配置

现在支持基于路由的细粒度策略配置：

```go
resilienceModule, err := resilience.ForRoot(resilience.Config{
    Strategy: resilience.StrategyConfig{
        RateLimit: map[string]resilience.RateLimitConfig{
            "user:create": {
                Enabled: true,
                Limit:   50,
                Window:  time.Minute,
            },
            "user:list": {
                Enabled: true,
                Limit:   200,
                Window:  time.Minute,
            },
        },
        Timeout: map[string]resilience.TimeoutConfig{
            "user:create": {
                Enabled:  true,
                Duration: 10 * time.Second,
            },
            "user:list": {
                Enabled:  true,
                Duration: 5 * time.Second,
            },
        },
        Retry: map[string]resilience.RetryConfig{
            "user:create": {
                Enabled:     true,
                MaxAttempts: 3,
                Backoff:     100 * time.Millisecond,
            },
        },
        Circuit: map[string]resilience.CircuitBreakerConfig{
            "user:create": {
                Enabled:          true,
                FailureThreshold: 3,
                OpenTimeout:      10 * time.Second,
            },
        },
    },
})
```

### 5.4 配置示例

完整的配置示例：

```go
resilienceModule, err := resilience.ForRoot(resilience.Config{
    Name:        "default",
    RateLimit: resilience.RateLimitConfig{
        Enabled: true,
        Limit:   100,
        Window:  time.Minute,
        Header:  "X-RateLimit-Key",
    },
    Timeout: resilience.TimeoutConfig{
        Enabled:  true,
        Duration: 5 * time.Second,
    },
    Retry: resilience.RetryConfig{
        Enabled:     true,
        MaxAttempts: 2,
        Backoff:     10 * time.Millisecond,
    },
    Circuit: resilience.CircuitBreakerConfig{
        Enabled:          true,
        FailureThreshold: 5,
        OpenTimeout:      30 * time.Second,
    },
    Idempotency: resilience.IdempotencyConfig{
        Enabled: true,
        Header:  "Idempotency-Key",
        TTL:     time.Minute,
    },
    DistributedStore: &RedisStore{client: redisClient},
    Strategy: resilience.StrategyConfig{
        RateLimit:  make(map[string]resilience.RateLimitConfig),
        Circuit:    make(map[string]resilience.CircuitBreakerConfig),
        Timeout:    make(map[string]resilience.TimeoutConfig),
        Retry:      make(map[string]resilience.RetryConfig),
        Idempotency: make(map[string]resilience.IdempotencyConfig),
    },
})
```
