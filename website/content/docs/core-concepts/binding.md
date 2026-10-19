---
title: Binding
weight: 5
prev: /docs/core-concepts/aop
next: /docs/core-concepts/errors-lifecycle
---

Binding 决定「请求数据如何变成 handler 参数」。每个协议都有自己的 wrapper 集合，但共享同一套解析框架。

## 1. 两种写法

### 1.1 直接 wrapper

```go
func (c *UserController) Get(args struct {
	ID   httpbinding.Param[int]     `param:"id" required:"true"`
	Trace httpbinding.Header[string] `header:"X-Trace-Id"`
}) (any, error) { ... }
```

### 1.2 `Body[T]` + validator

```go
type createUserRequest struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
}

type createUserArgs struct {
	Ctx  httpbinding.Context
	Body httpbinding.Body[createUserRequest]
}

func (c *UserController) Create(args createUserArgs) (any, error) {
	req := args.Body.Value
	...
}
```

需要 `httpadapter.New().WithValidatorV10()` 才会执行 `validate` 校验。

## 2. HTTP 常用 wrapper

| wrapper | 数据来源 | 说明 |
|---------|----------|------|
| `Context` | 请求上下文 | `Status` / `Header` / `JSON` / `Bind` |
| `Param[T]` | 路径参数 | `param:"id"` |
| `Query[T]` | 查询参数 | 单个字段或整份 query map |
| `Header[T]` | 请求头 | 单个字段或整份 header |
| `Body[T]` | JSON body | 自动执行 validator |
| `Form[T]` | 表单 | `application/x-www-form-urlencoded` |
| `Cookie[T]` / `Cookies` | Cookie | — |
| `File` / `Files` | 上传文件 | multipart |
| `Bind[T]` / `MustBind[T]` | 严格聚合绑定 | 只允许 body/param/query/header |

## 3. CompositeStruct

当一个 handler 需要多个来源时，把 wrapper 聚合进一个 struct：

```go
type listUsersArgs struct {
	Ctx     httpbinding.Context
	Page    httpbinding.Query[int]     `query:"page"`
	Size    httpbinding.Query[int]     `query:"size"`
	Keyword httpbinding.Query[string]  `query:"keyword"`
	TraceID httpbinding.Header[string] `header:"X-Trace-Id"`
}
```

识别规则：struct 里任意字段是「本包的 wrapper」即视为 CompositeStruct。

## 4. 跨协议对照

| 概念 | HTTP | gRPC | GraphQL | WS | MQ | Scheduler |
|------|------|------|---------|----|----|-----------|
| 请求体 | `Body[T]` | 请求 message / `Req[T]` | `Arg[T]` | `WSMessageBody[T]` | `Message[T]` | — |
| 上下文 | `Context` | `context.Context` | `Context` | `Context` | `Context` | `Context` |
| 头部 | `Header[T]` | `Header[T]` | `Header[T]` | `Header[T]` | `Header[T]` | — |
| 元信息 | `Param/Query` | `FullMethod/Service/Method` | `FieldName/Path` | `WSEvent/WSRooms` | `Topic/Consumer/Key` | `JobName/TriggerType/RunID` |

## 5. 解析顺序

```text
1. 按参数类型匹配协议 binding（后注册优先，且校验协议是否允许）
2. 命中则解析
3. 未命中则回退到 DI 容器
4. 仍未命中则返回 ErrBindingNotFound
```

多协议同进程时，binding 选择是**协议感知**的：
HTTP 的 `AutoStruct` 不会抢走 GraphQL 的 `AutoStruct`。

## 6. 常见问题

| 现象 | 原因 | 解决 |
|------|------|------|
| `binding not found` | 参数类型没有对应 resolver | 用 wrapper，或注册为 provider |
| `binding not supported for protocol` | 类型命中了别的协议的 binding | 检查是否用了错误协议的 wrapper |
| `validation failed on X` | validator 校验失败 | 检查 `validate` tag 与请求体 |
| 字段一直是零值 | wrapper 没有绑定名 | 补 `param:"..."` / `query:"..."` 等 tag |
