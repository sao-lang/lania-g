---
title: Binding
weight: 5
prev: /docs/reference/aop
next: /docs/reference/http
---

Binding 负责把请求数据变成 handler 参数。每个协议提供自己的 wrapper，但共享同一套解析框架。

## 1. 解析算法

```text
for i, paramType := range handler.Meta.ParamTypes:
    resolver, desc, ok := bindingRegistry.findForProtocol(paramType, ctx.Protocol)
    if ok:
        value = resolver.Resolve(ctx, desc)
    else:
        value = ctx.Container.Get(paramType)   // 回退 DI
    if still not found:
        return ErrBindingNotFound
```

`findForProtocol` 的关键点：

- 缓存 key 是 `(type, protocol)`，不是单一 type
- 遍历顺序「后注册优先」
- **类型匹配但协议不允许** → 跳过，继续找
- 全部被协议拒绝 → `ErrBindingNotSupported`

## 2. 全协议 wrapper 矩阵

### HTTP（`protocol/http/v3/binding`）

| wrapper | 数据源 | 备注 |
|---------|--------|------|
| `Context` | 请求上下文 | `Status/Header/JSON/Bind` |
| `Param[T]` | path 参数 | `param:"id"` |
| `Query[T]` | query | 单字段或整表 |
| `Header[T]` | header | 单字段或整表 |
| `Headers` | `map[string][]string` | 全部 header |
| `Body[T]` | JSON body | validator 生效 |
| `BodyAs[T]` | body（宽松） | — |
| `BodyBytes` / `MustBodyBytes` | 原始字节 | — |
| `Form[T]` | 表单 | validator 生效 |
| `Cookie[T]` / `Cookies` | Cookie | — |
| `File` / `Files` | 上传文件 | multipart |
| `Bind[T]` / `MustBind[T]` | 聚合严格绑定 | 仅 body/param/query/header |
| `Original` | `*http.Request` | — |
| `Next` | 下游 handler | mounted 场景 |
| `IP/Host/Method/Path/URL` | 请求元信息 | — |
| `UploadedFile` | 文件结构 | `Filename/Size/Header` |

认证相关（由 `integrations/auth` 注入）：

| wrapper | 含义 |
|---------|------|
| `AuthUser` / `AuthUserID` | 当前主体 |
| `AuthToken` / `AuthOptionalToken` | token |
| `AuthRole` / `AuthPermission` | 角色/权限 |
| `AuthOptionalUser` | 可选主体 |

### gRPC（`protocol/grpc/v3/binding`）

| wrapper | 数据源 |
|---------|--------|
| `Req[T]` | 请求消息 |
| 直接 proto message | 请求消息 |
| `Header[T]` | metadata（需 `BindParam`） |
| `Headers` / `Metadata` | 全部 metadata |
| `Context` | `context.Context` |
| `FullMethod/Service/Method` | RPC 元信息 |
| `RawServerStream` | 原始 stream |
| `ServerStream[T]` | 服务端发送 |
| `ClientStream[T]` | 客户端接收 |
| `BidiStream[Req,Resp]` | 双向收发 |
| `GRPCContext` | 增强上下文 |

### GraphQL（`protocol/graphql/v3/binding`）

| wrapper | 数据源 |
|---------|--------|
| `Arg[T]` / `ArgValue[T]` | 字段参数 |
| `Parent[T]` | 父节点 |
| `Header[T]` / `Headers` | 请求头 |
| `Context` | 增强上下文 |
| `Variables` / `Extensions` | 查询变量/扩展 |
| `SelectionSet` / `Root` / `Info` | 选择集与执行信息 |
| `OperationName` / `FieldName` / `RawQuery` | 查询元信息 |
| `IP/Host/Method/URL/Path/Session` | 请求元信息 |
| `Request` / `Response` | 原生 HTTP 对象 |

### WebSocket（`protocol/ws/v3/binding`）

| wrapper | 数据源 |
|---------|--------|
| `WSMessageBody[T]` | 事件消息体 |
| `Context` | 增强上下文 |
| `Header[T]` / `WSHeaders[T]` / `Headers` | 握手 header |
| `WSConnectedSocket` | 连接对象 |
| `WSEvent` / `WSSocketID` / `WSRooms` | 事件/连接信息 |

### MQ（`protocol/mq/v3/binding`）

| wrapper | 数据源 |
|---------|--------|
| `Message[T]` | 消息体 |
| `Header[T]` / `Headers` | 消息头（大小写无关） |
| `Topic` / `Consumer` / `Key` | 投递元信息 |
| `RetryCount` | 重试次数 |
| `Ack` / `Nack` | 确认回调 |

### Scheduler（`protocol/scheduler/v3/binding`）

| wrapper | 数据源 |
|---------|--------|
| `JobName` / `TriggerType` / `RunID` / `ScheduledAt` | 调度元信息 |
| `Context` | `context.Context` |

## 3. CompositeStruct

struct 中任意字段是本包 wrapper 时，整体按 CompositeStruct 逐字段解析：

```go
type args struct {
	Ctx     httpbinding.Context
	ID      httpbinding.Param[int]     `param:"id" required:"true"`
	Body    httpbinding.Body[CreateReq]
	TraceID httpbinding.Header[string] `header:"X-Trace-Id"`
}
```

### required 规则

| 字段类型 | `required:"true"` 语义 |
|----------|------------------------|
| `Param/Query/Header` | 必须存在且非空，且必须有绑定名 |
| `Body/Form` | 请求体/表单不能为空 |
| `File/Files` | 至少一个文件 |
| `Cookie` | 值不能为零值 |

## 4. 校验器

```go
type Validator interface {
	Validate(obj any) error
}

func NewValidatorV10() *V10Validator
```

- 使用 `validate:"..."` 标签
- 错误归一化为 `KernelError{Kind: Validation}`，`Meta.errors` 包含字段明细
- 生效范围：`Body[T]`、`Form[T]`、`AutoStruct`、`Bind[T]`、WS `ShouldBindMessage`

```go
httpAdapter := httpadapter.New().WithValidatorV10()
```

## 5. 严格/宽松绑定

| 函数 | 行为 |
|------|------|
| `BindInto(ctx, obj)` | 宽松：允许未识别字段 |
| `BindStrictInto(ctx, obj)` | 严格：未识别/无法映射字段报错 |
| `MustBindInto(ctx, obj)` | 出错 panic |

## 6. 排错

| 错误 | 含义 |
|------|------|
| `binding resolver not found for type` | 参数类型没有 resolver，也没在 DI 中 |
| `binding not supported for protocol` | 类型命中了其他协议的 resolver |
| `missing required param "id"` | required 字段缺失 |
| `validation failed on Email (email)` | validator 校验失败 |
