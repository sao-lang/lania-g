# lania-g v3 Outbox 接入

## 1. 提供能力

`integrations/outbox` 当前提供：

- outbox
- inbox
- 事务事件发布
- 重试与死信桥接

## 2. 基本接入

```go
busModule, _ := events.ForRoot(events.Config{})
bus := busModule.(*events.Module).Bus()

outboxModule, err := outbox.ForRoot(outbox.Config{
	Dispatcher: outbox.NewEventDispatcher(bus),
})
if err != nil {
	panic(err)
}
```

## 3. 核心能力

- `Publish(ctx, topic, payload)`
- `PublishInTransaction(ctx, manager, topic, payload)`
- `Flush(ctx, limit)`
- `Receive(ctx, message, handler)`

## 4. 组合方式

- 与 `events`
  - 通过 `NewEventDispatcher(...)`
- 与 `orm`
  - 通过 `PublishInTransaction(...)` 结合 `orm.TransactionManager`
- 与 `mq`
  - 通过自定义 `Dispatcher` 把消息刷到外部队列

## 5. 高级特性

### 5.1 持久化存储

现在支持多种持久化存储：

#### 5.1.1 MongoDB 存储

```go
// 创建 MongoDB 存储
mongoDB := client.Database("outbox")
mongoStore := outbox.NewMongoDBStore(mongoDB, "messages")

// 配置 Outbox
outboxModule, err := outbox.ForRoot(outbox.Config{
    Store:      mongoStore,
    Dispatcher: outbox.NewEventDispatcher(bus),
})
```

#### 5.1.2 SQL 存储

```go
// 创建 SQL 存储
sqlStore := outbox.NewSQLStore(db)

// 配置 Outbox
outboxModule, err := outbox.ForRoot(outbox.Config{
    Store:      sqlStore,
    Dispatcher: outbox.NewEventDispatcher(bus),
})
```

### 5.2 调度器

现在支持自动调度器，定期处理待发送的消息：

```go
outboxModule, err := outbox.ForRoot(outbox.Config{
    Store:      mongoStore,
    Dispatcher: outbox.NewEventDispatcher(bus),
    Scheduler: outbox.SchedulerConfig{
        Enabled:     true,
        Interval:    5 * time.Second,
        BatchSize:   100,
        Concurrency: 5,
    },
})
```

### 5.3 批处理与并发

现在支持批处理和并发处理消息：

- `BatchSize`：每次处理的消息数量
- `Concurrency`：并发处理的协程数量

### 5.4 增强的 Dead-Letter 管理

现在支持更强大的 dead-letter 管理：

```go
// 创建 dead-letter dispatcher
deadLetterDispatcher := outbox.NewEventDispatcher(deadLetterBus)

// 配置 Outbox
outboxModule, err := outbox.ForRoot(outbox.Config{
    Store:      mongoStore,
    Dispatcher: outbox.NewEventDispatcher(bus),
    DeadLetter: deadLetterDispatcher,
    MaxAttempts: 3,
})
```

### 5.5 高级 API

- `ReprocessDead(ctx, limit)`：重新处理死信消息
- `CleanupOldMessages(ctx, olderThan)`：清理旧的已处理消息

### 5.6 完整配置示例

```go
outboxModule, err := outbox.ForRoot(outbox.Config{
    Name:        "default",
    MaxAttempts: 3,
    Store:       mongoStore,
    Dispatcher:  outbox.NewEventDispatcher(bus),
    DeadLetter:  deadLetterDispatcher,
    Scheduler: outbox.SchedulerConfig{
        Enabled:     true,
        Interval:    5 * time.Second,
        BatchSize:   100,
        Concurrency: 5,
    },
})
```

### 5.7 与 MongoDB 集成

```go
// 创建 MongoDB 存储
mongoModule, _ := mongodb.ForRoot(mongodb.Config{
    URI:      "mongodb://localhost:27017",
    Database: "outbox",
})
mongoDB := mongoModule.(*mongodb.Module).DB()
mongoStore := outbox.NewMongoDBStore(mongoDB, "messages")

// 配置 Outbox
outboxModule, err := outbox.ForRoot(outbox.Config{
    Store:      mongoStore,
    Dispatcher: outbox.NewEventDispatcher(bus),
})
```

### 5.8 与 SQL 集成

```go
// 创建 SQL 存储
ormModule, _ := orm.ForRoot(orm.Config{
    DSN: "mysql://user:pass@localhost:3306/outbox",
})
db := ormModule.(*orm.Module).DB()
sqlStore := outbox.NewSQLStore(db)

// 配置 Outbox
outboxModule, err := outbox.ForRoot(outbox.Config{
    Store:      sqlStore,
    Dispatcher: outbox.NewEventDispatcher(bus),
})
```
