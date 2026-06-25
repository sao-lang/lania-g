# gRPC 四种模式支持实现说明

## 1. 文档目的

本文档用于说明 `lania-g` 当前 gRPC 四种调用模式的实现现状、公开能力以及主要设计取舍。

本文档重点覆盖以下内容：

- 当前 gRPC 服务端适配器的能力现状
- gRPC 四种模式在 DSL / transport / binding / demo 中的落地形态
- `integrations/grpc/v3` 客户端 integration 当前公开的 streaming 接入方式
- gRPC 自动绑定类型、增强上下文与 DTO 绑定入口
- 当前仍保留的实现边界与注意事项

本文档面向框架维护者、协议适配器开发者以及需要参与 gRPC 能力扩展的同学。

## 2. 当前现状

### 2.1 当前已支持四种调用模式

当前仓库中的 gRPC 服务端适配器位于：

- `protocol/grpc/v3`

其主要能力包括：

- 通过 DSL 声明 gRPC service / method
- 在编译阶段将方法声明编译为 runtime handler
- 在运行阶段动态注册 `grpc.ServiceDesc`
- 将一次 gRPC 请求桥接为统一的 `runtime.HandlerContext`
- 复用现有 binding / AOP / DI / runtime 执行链路
- 支持 unary / server streaming / client streaming / bidirectional streaming 四种模式

当前公开的 DSL 入口包括：

- `Method(...)`
- `ServerStreamMethod(...)`
- `ClientStreamMethod(...)`
- `BidiStreamMethod(...)`

因此，当前能力本质上是：

- 支持 `Unary RPC`
- 支持 `Server Streaming RPC`
- 支持 `Client Streaming RPC`
- 支持 `Bidirectional Streaming RPC`

### 2.2 当前服务端适配器的主要形态

当前 `protocol/grpc/v3` 的运行时形态主要是：

- unary 继续以单个 request message 为主路径
- server streaming 保留首个 request message，并为 handler 注入 `ServerStream[T]`
- client streaming / bidi streaming 通过 `Request.Raw` 承接底层 `grpc.ServerStream`
- binding 层提供 `RawServerStream`、`ServerStream[T]`、`ClientStream[T]`、`BidiStream[Req, Resp]`
- 增强上下文 `GRPCContext` 提供 `ShouldBindReq()` 与 `ShouldBindStream()` 做 DTO 绑定和校验

需要注意的边界：

- `Req[T]` 和直接 request message 只适用于 unary 与 server streaming 首请求
- `ShouldBindReq(&dto)` 只适用于 unary 与 server streaming 首请求
- `ShouldBindStream(msg, &dto)` 只适用于 client streaming 与 bidi streaming 的单条消息
- 更底层的流式能力仍可通过 `RawServerStream` 暴露给业务 handler

### 2.3 当前客户端 integration 形态

当前 gRPC 客户端 integration 位于：

- `integrations/grpc/v3`

其当前能力主要是：

- 创建并维护 `*grpc.ClientConn`
- 暴露默认 client、命名 client、连接与 factory
- 通过 DI 注入客户端连接能力
- 提供 `Client.Invoke(...)` 作为直接调用入口
- 提供 `Client.NewStream(...)` / `Factory.NewStream(...)` 作为 streaming 调用入口

当前 integration 仍保持“基础连接 + 原生 gRPC stream”的设计，不额外包装更高层 helper：

- unary 继续通过 `Invoke(...)` 调用
- streaming 通过 `NewStream(...)` 配合 `grpc.StreamDesc` 发起
- 当前没有额外提供 `InvokeServerStream(...)`、`InvokeClientStream(...)`、`InvokeBidiStream(...)` 这类更高阶 helper

这意味着框架已经具备四种模式的服务端能力与基础客户端接入能力，但客户端 streaming 仍更偏原生 gRPC 风格。

## 3. 目标

### 3.1 当前公开能力

当前框架已经公开支持以下四种 gRPC 调用模式：

1. `Unary RPC`
2. `Server Streaming RPC`
3. `Client Streaming RPC`
4. `Bidirectional Streaming RPC`

这里的“支持”具体指：

- DSL 层可以声明四种模式
- 编译层可以为四种模式生成正确的运行时产物
- transport 层可以将四种模式注册到底层 `grpc.Server`
- runtime 可以复用现有执行模型承接这四种模式
- 自动绑定层可以为四种模式提供清晰、稳定、可预测的参数注入能力
- integration 层可以为客户端侧提供 unary `Invoke(...)` 与基础 `NewStream(...)` 能力
- 文档、示例、测试与诊断信息可以体现完整能力

### 3.2 非目标

当前实现仍然不以“重写 runtime”为目标，也不把 gRPC streaming 改造扩散到所有协议层。

明确不作为本轮目标的事项包括：

- 不推翻现有 `runtime.HandlerContext` / `Executor` / `Pipeline` 主模型
- 不为了 gRPC streaming 修改 HTTP / WS / MQ 的参数绑定语义
- 不要求所有业务 handler 统一改成流式接口
- 不要求 compat 入口在本轮被完全删除

## 4. 设计原则

本方案遵循以下原则：

1. 保留 unary 现有主路径，避免无关回归。
2. 新增 streaming 能力时优先在 gRPC adapter 与 gRPC binding 内局部扩展，不侵入 runtime 主流程。
3. 对业务签名保持显式语义，避免把多消息流伪装成单个 request body。
4. 区分“上下文类注入”和“消息流类注入”，避免自动绑定语义混乱。
5. 服务端适配器与客户端 integration 共同演进，保证框架层对四种模式的支持是一致的，而不是只有服务端半成品能力。

## 5. 总体方案

### 5.1 分层改造范围

本次改造涉及两条主线：

- 服务端协议层：`protocol/grpc/v3`
- 客户端 integration 层：`integrations/grpc/v3`

其中服务端协议层包括：

- DSL
- method definition
- protocol plugin
- transport registration
- binding types
- binding resolvers
- 测试与 demo

客户端 integration 层包括：

- client API
- factory API
- DI 注入与 refs
- 文档与示例

### 5.2 runtime 复用策略

当前 runtime 已具备以下可复用前提：

- `HandlerContext.Request.Raw` 可保存协议原生对象
- binding 机制不要求参数必须来自 `Request.Body`
- handler 执行仍然可以被 AOP / DI / filters 复用

因此，本方案不建议重构 runtime 主模型，而建议采用以下策略：

- unary 继续以 `Request.Body` 作为主请求对象
- streaming 模式将 `grpc.ServerStream` 或其包装对象放入 `Request.Raw`
- 自动绑定通过新增的 gRPC stream wrapper 从 `Request.Raw` 解析能力
- handler 内部通过 wrapper 显式完成 `Recv` / `Send`

这样可以把流式扩展主要控制在 gRPC 适配器内部。

## 6. 服务端适配器改造

### 6.1 DSL 改造

当前 `Method(...)` 仅表示 unary。完整支持四种模式后，DSL 需要能够表达调用模式。

建议方案如下：

- 保留 `Method(...)` 表示 `Unary RPC`
- 新增 `ServerStreamMethod(...)`
- 新增 `ClientStreamMethod(...)`
- 新增 `BidiStreamMethod(...)`

也可以选择更统一的 builder 风格，但从可读性与迁移成本考虑，建议优先采用显式命名 API。

示意：

```go
grpcAPI.Service("UserService", svc).
    Method("GetUser", svc.GetUser).
    ServerStreamMethod("WatchUsers", svc.WatchUsers).
    ClientStreamMethod("UploadUsers", svc.UploadUsers).
    BidiStreamMethod("ChatUsers", svc.ChatUsers).
    Build()
```

设计理由：

- 保持 unary 调用路径不变
- 让 streaming 语义在声明期就显式可见
- 降低后续诊断、代码审查与文档示例的理解成本

### 6.2 MethodDefinition 改造

当前 `MethodDefinition` 只面向 unary，需要补充流式能力描述。

建议新增字段包括：

- `Mode`
  - 标识调用模式，例如 `unary` / `server_stream` / `client_stream` / `bidi_stream`
- `RequestType`
  - 保留，用于 unary 与 server-stream 首消息解码
- `ResponseType`
  - 新增，用于 stream wrapper 的类型信息与运行期校验
- `ClientStreams`
  - 可选，如果希望兼容底层 `grpc.StreamDesc` 建模
- `ServerStreams`
  - 可选，如果希望兼容底层 `grpc.StreamDesc` 建模

其中推荐以 `Mode` 作为框架内部主语义字段，再在 transport 注册阶段派生底层 `ClientStreams` / `ServerStreams` 标记。

### 6.3 transport 注册改造

当前 transport 注册只构造 `grpc.MethodDesc`。要支持四种模式，必须扩展为同时构造：

- `grpc.MethodDesc`
- `grpc.StreamDesc`

建议策略如下：

- `Unary RPC`
  - 继续注册到 `ServiceDesc.Methods`
- `Server Streaming RPC`
  - 注册到 `ServiceDesc.Streams`
  - `ServerStreams = true`
- `Client Streaming RPC`
  - 注册到 `ServiceDesc.Streams`
  - `ClientStreams = true`
- `Bidirectional Streaming RPC`
  - 注册到 `ServiceDesc.Streams`
  - `ClientStreams = true`
  - `ServerStreams = true`

同时需要新增：

- `makeStreamHandler(...)`
- `dispatchStream(...)`

#### 6.3.1 Unary 保持现状

对于 unary：

- 继续走当前 `dec(in)` 解码单个请求
- 继续复用 `dispatchUnary(...)`
- 继续兼容现有 unary interceptor 行为

#### 6.3.2 Server Streaming

对于 server streaming：

- transport 先读取首个请求消息
- 将首请求写入 `Request.Body`
- 将底层 `grpc.ServerStream` 写入 `Request.Raw`
- handler 内部通过 stream wrapper 持续 `Send(...)`

这种模式下可以复用现有 request message 注入能力。

#### 6.3.3 Client Streaming

对于 client streaming：

- transport 不再假设只有一个请求消息
- `Request.Body` 不作为主输入来源
- 底层 `grpc.ServerStream` 写入 `Request.Raw`
- handler 通过 stream wrapper 循环 `Recv(...)`
- handler 结束后由 transport 将单个响应写回客户端

#### 6.3.4 Bidirectional Streaming

对于 bidi streaming：

- transport 将底层 `grpc.ServerStream` 写入 `Request.Raw`
- handler 通过 wrapper 同时完成 `Recv(...)` 与 `Send(...)`
- handler 的主返回值以 `error` 为主

### 6.4 签名推断与校验改造

当前请求类型推断围绕 unary 签名设计。支持四种模式后，推断逻辑应分模式处理。

建议规则：

- `Unary RPC`
  - 保持当前规则
- `Server Streaming RPC`
  - 允许推断首个 request message
  - 同时要求签名中存在 stream 参数
- `Client Streaming RPC`
  - 不再依赖 `RequestType` 自动推断单次请求
  - 要求签名中存在 client-stream wrapper
- `Bidirectional Streaming RPC`
  - 要求签名中存在 bidi-stream wrapper

建议同时引入更强的启动前诊断：

- 缺少 streaming wrapper 时直接报错
- unary 方法中误用了 stream wrapper 时直接报错
- client-stream / bidi-stream 仍声明 `Req[T]` 时直接报错

### 6.5 服务端示例签名

建议业务 handler 签名形态如下：

```go
type GetUserArgs struct {
    Req    *pb.GetUserRequest      `req:"true" required:"true"`
    Meta   grpcbinding.Metadata
    Method grpcbinding.FullMethod
}

func (s *UserService) GetUser(args GetUserArgs) (*pb.GetUserResponse, error)
```

```go
func (s *UserService) WatchUsers(
    req *pb.WatchUsersRequest,
    stream grpcbinding.ServerStream[*pb.UserEvent],
) error
```

```go
func (s *UserService) UploadUsers(
    stream grpcbinding.ClientStream[*pb.UploadUserChunk],
) (*pb.UploadUsersResult, error)
```

```go
func (s *UserService) ChatUsers(
    stream grpcbinding.BidiStream[*pb.ChatUsersRequest, *pb.ChatUsersResponse],
) error
```

说明：

- 本轮实现中，unary 推荐把请求消息、metadata 与方法信息收口到 `CompositeStruct`
- streaming 场景下也允许把 `RawServerStream` / `ServerStream[T]` / `ClientStream[T]` / `BidiStream[...]` 放进 `CompositeStruct`
- `req:"true"` / `header:"..."` / `required:"true"` 用于显式声明字段来源
- 直接 request message 参数与 `Req[T]` 仍保留为兼容写法，但不再作为主示例

## 7. 客户端 integration 改造

### 7.1 为什么 integration 也需要改

如果只改服务端适配器，而不改客户端 integration，那么框架层对 gRPC streaming 的支持将停留在：

- 服务端有 DSL 和 binding
- 客户端仍需业务自己通过 `Conn()` 写原生流式代码

这种状态不适合作为“框架完整支持 gRPC 四种模式”的对外结论。

因此，`integrations/grpc/v3` 也需要改造，使其从“仅封装 unary invoke”升级为“对四种模式都有统一接入面”。

### 7.2 Client API 改造

当前 `Client` 主要公开：

- `Invoke(...)`
- `Conn()`
- `Close()`

建议补充以下能力：

- `NewStream(...)`
  - 暴露原生 stream 创建能力
- `InvokeServerStream(...)`
  - 封装 server streaming 调用
- `InvokeClientStream(...)`
  - 封装 client streaming 调用
- `InvokeBidiStream(...)`
  - 封装 bidi streaming 调用

如果担心 API 过重，也可以保留为两层：

- 低层：`NewStream(...)`
- 高层：可选 helper

推荐最小落地方式：

1. 必做：新增 `NewStream(...)`
2. 选做：根据业务使用频率补 `InvokeServerStream(...)` / `InvokeClientStream(...)` / `InvokeBidiStream(...)`

### 7.3 Factory 与 Config 改造

当前 `Factory` 主要用于创建 client 与连接。若引入 streaming helper，建议同步扩展 `Factory` 能力，至少保证：

- `Factory` 可创建支持 streaming 的 client
- `Client` 默认 call options 仍能参与 streaming 调用

`Config` 不一定需要大改，但建议考虑是否补充：

- 流式调用默认 call options
- 流式调用相关 interceptor 配置
- 是否启用默认消息大小限制或 keepalive 策略

这些项可以作为后续增强项，不必成为首轮硬要求。

### 7.4 refs 与自动注入

当前 integration 已支持：

- 默认 client 注入
- 命名 client 注入
- 默认 conn 注入
- 命名 conn 注入

这套注入模型可以继续保留，不需要因为 streaming 支持而重做 token 体系。

但文档与示例应明确说明：

- unary 场景可直接注入 `*Client` 调 `Invoke(...)`
- streaming 场景可注入 `*Client` 或 `*grpc.ClientConn` 调用 `NewStream(...)` 或对应 helper

换言之，integration 的 DI 注入模型可复用，主要变化在于 client API 的扩展，而不是 token 模型的重建。

## 8. 自动绑定类型改造

### 8.1 设计原则

gRPC 自动绑定类型的改造应遵循以下原则：

1. 保留 unary 现有绑定体验。
2. 不把“消息流”伪装成单个 request body。
3. 上下文类信息继续自动注入，消息流类能力通过新增 wrapper 显式注入。
4. 不让 client-stream / bidi-stream 的 handler 签名出现含糊语义。

### 8.2 现有绑定类型的处理策略

当前 gRPC binding 中已有的类型包括：

- `Req[T]`
- `Header[T]`
- `Metadata`
- `Headers`
- `Context`
- `FullMethod`
- `Service`
- `Method`

建议处理策略如下：

#### 8.2.1 保留不变

以下类型可直接保留并复用于四种模式：

- `Header[T]`
- `Metadata`
- `Headers`
- `Context`
- `FullMethod`
- `Service`
- `Method`

原因是这些类型表达的是上下文、metadata 与路由信息，不依赖“单个 request body”的假设。

#### 8.2.2 保留但限制适用范围

以下类型应保留，但限制为 unary 或 server-stream 的首请求场景：

- `Req[T]`
- `RequestMessage`

建议规则：

- `Unary RPC`
  - 可使用
- `Server Streaming RPC`
  - 可用于首个 request message
- `Client Streaming RPC`
  - 禁止使用
- `Bidirectional Streaming RPC`
  - 禁止使用

原因是 client-stream 与 bidi-stream 不再存在“唯一主请求消息”。

### 8.3 需要新增的绑定类型

为了支撑 streaming，建议新增以下 wrapper 类型。

#### 8.3.1 原生流包装

```go
type RawServerStream struct {
    grpc.ServerStream
}
```

用途：

- 暴露最原始的流能力
- 便于高级场景直接访问底层 gRPC API

#### 8.3.2 ServerStream 响应流包装

```go
type ServerStream[T any] interface {
    Send(msg T) error
}
```

用途：

- 用于 server streaming
- handler 只关心发送响应消息

#### 8.3.3 ClientStream 请求流包装

```go
type ClientStream[T any] interface {
    Recv() (T, error)
}
```

用途：

- 用于 client streaming
- handler 只关心接收请求消息

#### 8.3.4 BidiStream 双向流包装

```go
type BidiStream[Req any, Resp any] interface {
    Recv() (Req, error)
    Send(msg Resp) error
}
```

用途：

- 用于双向流
- 显式表达双向收发能力

### 8.4 为什么建议新增而不是复用现有类型

不建议尝试用 `Req[T]` 或 `RequestMessage` 去兼容流式场景，原因包括：

- 语义不清晰
- handler 签名会误导使用者，以为只有单个请求对象
- binding resolver 需要隐式决定“当前拿首条消息还是当前消息还是整个流”，可预测性很差
- 错误处理和 AOP 语义会变得不稳定

因此，新增 streaming wrapper 是更清晰的做法。

### 8.5 binding resolver 改造

gRPC binding resolver 需要新增对以下类型的匹配与解析：

- `RawServerStream`
- `ServerStream[T]`
- `ClientStream[T]`
- `BidiStream[Req, Resp]`

解析策略建议如下：

- transport 在 streaming 场景下把底层 `grpc.ServerStream` 写入 `HandlerContext.Request.Raw`
- resolver 从 `Request.Raw` 中读取该对象
- resolver 根据 wrapper 类型返回适配后的包装对象

同时保留现有 resolver：

- `Context`
- `Metadata`
- `Header[T]`
- `FullMethod`
- `Service`
- `Method`

### 8.6 参数绑定适用矩阵

建议明确建立 gRPC 参数绑定适用矩阵：

| 绑定类型 | Unary | Server Streaming | Client Streaming | Bidi Streaming |
| --- | --- | --- | --- | --- |
| `Req[T]` | 支持 | 支持首请求 | 不支持 | 不支持 |
| `RequestMessage` | 支持 | 支持首请求 | 不支持 | 不支持 |
| `Header[T]` | 支持 | 支持 | 支持 | 支持 |
| `Metadata` / `Headers` | 支持 | 支持 | 支持 | 支持 |
| `Context` | 支持 | 支持 | 支持 | 支持 |
| `FullMethod` / `Service` / `Method` | 支持 | 支持 | 支持 | 支持 |
| `RawServerStream` | 不推荐 | 支持 | 支持 | 支持 |
| `ServerStream[T]` | 不支持 | 支持 | 不支持 | 不支持 |
| `ClientStream[T]` | 不支持 | 不支持 | 支持 | 不支持 |
| `BidiStream[Req, Resp]` | 不支持 | 不支持 | 不支持 | 支持 |

### 8.7 绑定错误与诊断

新增 streaming binding 后，建议补充以下诊断规则：

- `Unary RPC` 使用 `ServerStream[T]` / `ClientStream[T]` / `BidiStream[...]` 时直接报错
- `Client Streaming RPC` 仍声明 `Req[T]` 或 request message 参数时直接报错
- `Bidi Streaming RPC` 缺少 bidi stream wrapper 时直接报错
- `Server Streaming RPC` 缺少 response stream wrapper 时直接报错
- 单个 handler 混用多个冲突的 stream wrapper 时直接报错

这些错误应尽量在编译阶段或启动前诊断阶段暴露，而不是等到运行时才失败。

## 9. 兼容性策略

### 9.1 对现有 unary 用户的兼容

本方案必须保证：

- 现有 `Method(...)` 不需要改名
- 现有 unary handler 签名继续可用
- 现有 `Req[T]` / `Header[T]` / `Metadata` 等 unary 绑定继续可用
- 现有 `Client.Invoke(...)` 继续保留

也就是说，新增 streaming 能力不能破坏当前 unary 主路径。

### 9.2 对旧 DSL 的兼容

新增 DSL 时建议：

- 不改变 `Method(...)` 语义
- 将 streaming 能力通过新增 API 暴露
- 在文档中把 streaming API 作为增强能力明确标注

### 9.3 对 integration 的兼容

客户端 integration 的兼容策略建议为：

- 保留 `Conn()` 逃生口
- 保留 `Invoke(...)`
- 增量增加 `NewStream(...)` 或更高层 helper

这样旧用户不会因为升级而中断，新的 streaming 用户也能使用框架主路径。

## 10. 实施顺序

建议实施顺序如下：

1. 改造 `MethodDefinition` 与 DSL，建立四种模式的声明模型。
2. 改造服务端 transport，使其支持 `MethodDesc` 与 `StreamDesc` 的双注册。
3. 新增 gRPC streaming binding types 与 resolver。
4. 补充编译期校验、启动前诊断与错误提示。
5. 扩展 `integrations/grpc/v3` 的 client API。
6. 补充 demo、测试与文档。

原因是：

- 第 1 到第 3 步构成服务端最小可闭环
- 第 4 步负责提升可预测性
- 第 5 步负责补齐“框架完整支持”的客户端侧承诺
- 第 6 步负责把能力真正交付给使用者

## 11. 测试与文档要求

### 11.1 服务端测试

至少应覆盖：

- unary 路径不回归
- `ServiceDesc.Streams` 正确注册
- `ServerStreams` / `ClientStreams` 标志正确
- streaming wrapper 可以从 binding 正确解析
- client-stream 的单响应返回行为正确
- bidi-stream 的收发循环与错误传播正确

### 11.2 integration 测试

至少应覆盖：

- `Client.NewStream(...)` 或等效 helper 可正确创建流
- 默认 call options 在 streaming 场景下仍可生效
- 通过默认 client / 命名 client 注入后可以发起 streaming 调用

### 11.3 文档与示例

需要同步更新：

- gRPC adapter 文档
- gRPC integration 文档
- 协议绑定与集成使用指南中的 gRPC 章节
- demo 示例

文档中必须明确：

- 四种模式各自推荐的 handler 签名
- 哪些绑定类型是 unary 专属
- 哪些绑定类型是 streaming 新增能力

## 12. 风险与注意事项

### 12.1 语义混淆风险

最大的风险不是 transport 本身，而是自动绑定语义被做得过于隐式，导致业务难以判断：

- 某个参数到底来自首条消息还是整个流
- 某种 wrapper 是否适用于当前模式
- 某个错误是在 binding 阶段还是消息收发阶段触发

因此必须坚持：

- 上下文类绑定继续自动
- 流能力类绑定必须显式

### 12.2 AOP 语义边界

当前 runtime / pipeline 是围绕“调用一个 handler”建模的。对于 streaming：

- AOP 作用点应仍以“进入 handler”这一层为主
- handler 内部多次 `Recv` / `Send` 不应被伪装成多次独立 runtime 调用

这意味着首轮改造中，不建议把“每条流消息都重新走一遍 runtime pipeline”作为目标。

### 12.3 诊断复杂度

streaming 支持后，编译与启动前诊断会变复杂，因此要尽量把模式校验前置到声明与编译阶段，而不是依赖运行时报错。

## 13. 一句话结论

如果框架要对外宣称“完整支持 gRPC 四种模式”，则必须同时完成以下三件事：

1. 改造 `protocol/grpc/v3` 服务端适配器，使其支持四种模式的 DSL、编译、transport 注册与运行期桥接。
2. 改造 `integrations/grpc/v3` 客户端 integration，使其在框架层提供 streaming 调用能力，而不只是暴露原生连接。
3. 改造 gRPC 自动绑定体系，保留 unary 绑定并新增 streaming wrapper 类型，明确四种模式下的适用边界与诊断规则。

只有这三部分同时完成，框架层的 gRPC 支持才算真正闭环。
