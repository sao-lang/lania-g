# 分层规则

这个框架同时提供 **AOP 层**和 **Transport 层**两套切入点。规则只有一句：

> **能进 runtime 的一律走 AOP；必须早于路由匹配的，才留在 Transport，并且必须在配置里显式标明。**

## 1. 决策矩阵

| 我要做的事 | 放在哪 | 为什么 |
|-----------|--------|--------|
| 读请求头做鉴权 | **AOP Guard** | 需要按控制器/路由分级，且 6 种协议通用 |
| 权限校验 | **AOP Guard** | 同上 |
| 改响应体形态（统一 envelope） | **AOP Interceptor** | 需要包裹 handler 返回值 |
| 错误 → 统一错误体 | **AOP ExceptionFilter** | 需要跨协议 |
| 参数清洗 / 类型转换 / 校验 | **AOP Pipe** | 需要参数级粒度 |
| 访问日志 / 耗时统计 | **AOP Middleware** | 见 `runtime/enhancers.AccessLog` |
| 请求 ID | **AOP Middleware** | 见 `runtime/enhancers.RequestID` |
| 超时 | **AOP Middleware** | 见 `runtime/enhancers.Timeout` |
| 跨协议链路追踪 | **AOP Middleware** | 见 `integrations/otel` |
| 限流 / 熔断 / 重试 | **AOP**（见 `integrations/resilience`） | 需要访问执行语义 |
| **CORS preflight 短路** | **Transport**（`http.Config.CORS`） | 必须在路由匹配前返回 `204`，不能进 runtime |
| **安全响应头** | **Transport**（`http.Config.Helmet`） | 同上，且要覆盖 404 等未命中路由 |
| **请求体大小限制** | **Transport**（`http.Config.MaxBodyBytes`） | 需要在读取 body 前拒绝 |
| **给 mounted handler 加逻辑** | **Transport**（`http.Config.PreRouting`） | swagger / 静态资源不走 runtime |

## 2. 为什么不能全放 AOP

三个例子说明 Transport 层的必要性：

1. **CORS preflight**：浏览器发送 `OPTIONS` 且不带业务参数。如果走 runtime，
   会先匹配路由、解析参数，然后才"发现"这是 preflight——参数解析可能先报错。
2. **mounted handler**：`/docs`（swagger）、静态资源由 adapter 直接 serve，
   **根本不经过 runtime**，AOP 对其完全无效。
3. **body 限制**：必须在 `json.Decode` 之前拒绝，否则内存已经被读完了。

## 3. 命名约定

Transport 层的每个配置项都必须能一眼看出"执行于路由之前"：

| 名字 | 含义 |
|------|------|
| `Config.PreRouting` | 路由匹配前执行的 HTTP 中间件 |
| `Config.CORS` / `Config.Helmet` | 传输层协议语义 |
| `Config.MaxBodyBytes` | 传输层限制 |
| `Config.NotFound` | 路由未命中兜底 |

禁止出现不带前缀的 `Middlewares` 字段——它无法表达执行时机（历史遗留已重命名）。

## 4. 两层的执行顺序

```text
HTTP 请求
  │
  ├─ Transport: Helmet（写响应头）
  ├─ Transport: CORS（preflight 可能在这里 204 结束）
  ├─ Transport: basePath 剥离
  ├─ Transport: mounted handler 匹配（swagger/静态资源，命中即结束）
  ├─ Transport: body 大小限制 + 解码
  │
  ├─ Runtime: 路由匹配
  ├─ AOP Middleware（global → scoped → controller → route）
  ├─ AOP Guard
  ├─ AOP Interceptor
  ├─ AOP Pipe（入参）
  ├─ Handler
  ├─ AOP Pipe（返回值）
  ├─ AOP Interceptor（返回）
  ├─ AOP Guard（返回）
  └─ AOP Middleware（返回）
       ↓
     无错误：Transport 写响应
     有错误：AOP ExceptionFilter（可能消费）→ 否则 Transport 写统一错误体
```

注意最后一步：**ExceptionFilter 消费成功后，响应仍由 Transport 统一写出**。
所以 filter 应该写 `runtime.Response`（Status/Body/Headers），而不是直接操作 `http.ResponseWriter`。

## 5. 检查清单

新增一个横切能力时，按顺序问：

1. 它是否必须早于路由匹配？（是 → Transport，且要覆盖 mounted handler）
2. 它是否需要跨协议？（是 → AOP，不能放 Transport）
3. 它是否需要按控制器/路由分级？（是 → AOP）
4. 它是否只改 HTTP 的 wire 语义？（是 → Transport）
5. 都不确定 → **默认 AOP**（可迁移性最好）
