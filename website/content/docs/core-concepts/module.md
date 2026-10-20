---
title: Module
weight: 1
prev: /docs/getting-started/rest-service
next: /docs/core-concepts/di
---

模块是 `lania-g` 的组织单元：它同时描述**依赖关系**和**归属关系**。

## 1. 模块的四个槽位

```go
func New(Metadata) *BaseModule
func AsGlobal(Metadata) *GlobalModule

type Metadata struct {
	Imports     []Module      // 子模块
	Providers   []*di.Provider // 本模块提供的实例
	Controllers []any          // 协议 controller / gateway / consumer / job
	Resolvers   []any          // GraphQL resolver
	Exports     []any          // 向 imports 方暴露的 token
	IsGlobal    bool
	AOP         AOP            // 模块贡献的全局 AOP（等价 Nest APP_*）
}
```

| 槽位 | 用途 | 典型协议 |
|------|------|----------|
| `providers` | 注册可注入实例 | 全部 |
| `controllers` | 声明 handler 的 owner | HTTP / gRPC / WS / MQ / Scheduler |
| `resolvers` | GraphQL 字段归属 | GraphQL |
| `exports` | 允许父级模块注入 | 全部 |

## 2. 根模块与子模块

```go
storeModule := NewStoreModule()

root := module.New(module.Metadata{
	Imports:     []module.Module{storeModule},
	Controllers: []any{authCtrl, userCtrl},
})
```

加载顺序：

```text
根模块 → 递归 imports（深度优先） → 每个模块 Init()
```

## 3. 导入与导出的可见性

- `exports` 里的 token 才能被 `imports` 方通过容器解析
- 未导出的 provider 只在**模块内部容器**可见
- 子模块之间互不可见，必须经过父模块或共同祖先

```go
// StoreModule
Exports: []any{reflect.TypeFor[*MemoryStore]()}
```

## 4. 生命周期钩子

模块可以实现以下接口（按需，不强制）：

```go
type OnModuleInit interface{ OnModuleInit() error }
type OnModuleDestroy interface{ OnModuleDestroy() error }
type OnApplicationBootstrap interface{ OnApplicationBootstrap() error }
type OnApplicationShutdown interface{ OnApplicationShutdown() error }
```

| 钩子 | 时机 | 典型用途 |
|------|------|----------|
| `OnModuleInit` | 模块容器构建完成后 | 建立连接、初始化内部资源 |
| `OnApplicationBootstrap` | 所有 adapter 启动前 | 桥接逻辑、事件挂载 |
| `OnModuleDestroy` | 应用停止时 | 关闭连接 |
| `OnApplicationShutdown` | 全部 adapter 停止后 | 释放全局资源 |

## 5. 自定义模块示例

```go
type CacheModule struct {
	*module.BaseModule
	cache *Cache
}

func NewCacheModule() *CacheModule {
	cache := NewCache()
	p, _ := di.ProviderFromValue[*Cache](cache, di.Singleton)
	return &CacheModule{
		BaseModule: module.New(module.Metadata{
			Providers: []*di.Provider{p},
						Exports:   []any{di.TokenFor[*Cache]()},
		}),
		cache: cache,
	}
}

func (m *CacheModule) OnModuleInit() error { return m.cache.Warmup() }
func (m *CacheModule) OnModuleDestroy() error { return m.cache.Close() }
```

## 6. 常见错误

| 现象 | 原因 |
|------|------|
| `provider not found` | provider 没注册，或没有 `exports` |
| `circular dependency` | A 依赖 B，B 又依赖 A |
| `module owner not found` | controller 没放进 `controllers` 槽位 |
| GraphQL 字段解析不到 | receiver 没放进 `resolvers` 槽位 |
