---
title: 错误模型
weight: 13
prev: /docs/reference/integrations
---

包：`github.com/sao-lang/lania-g/kernel/v3/errors`

框架用**一个结构化错误 + 一组哨兵错误 + 一层协议映射**统一处理所有失败路径。

## 1. KernelError

```go
type KernelError struct {
	Kind       Kind
	Protocol   string
	RouteKey   string
	ModuleKey  string
	ParamIndex int
	Message    string
	Cause      error
	Meta       map[string]interface{}
}
```

| 字段 | 用途 |
|------|------|
| `Kind` | 高层分类，adapter 用它决定状态码 |
| `Protocol` | 出错协议（http/grpc/graphql/ws/mq/scheduler） |
| `RouteKey` | 定位到具体路由 |
| `ModuleKey` | 定位到模块 |
| `ParamIndex` | 出错的参数下标（`-1` 表示不适用） |
| `Message` | 对外可读消息（`Error()` 优先返回它） |
| `Cause` | 原始错误，`errors.Is/As` 可穿透 |
| `Meta` | 结构化诊断（`stage`、`bindingName`、`paramType`、`errors` 等） |

## 2. Kind 分类

| Kind | 语义 | HTTP | gRPC（配 interceptor） |
|------|------|------|------------------------|
| `KindRouteNotFound` | 路由未命中 | 404 | `NotFound` |
| `KindBinding` | 参数绑定/转换失败 | 400 | `InvalidArgument` |
| `KindValidation` | 校验失败 | **422** | `InvalidArgument` |
| `KindDI` | 依赖解析失败 | 400 | `InvalidArgument` |
| `KindUnauthorized` | 未认证 | 401 | `Unauthenticated` |
| `KindForbidden` | 已识别但被拒 | 403 | `PermissionDenied` |
| `KindExecution` | 执行链失败（panic/异常） | 500 | `Internal` |
| `KindUnknown` | 未分类 | 500 | `Unknown` |

`kerrors.Classify(err)` 可以直接拿到分类结果：

```go
kind := kerrors.Classify(err) // 未命中任何规则时返回 KindUnknown
```

## 2.1 HTTP 统一错误响应

HTTP adapter 把所有框架错误写成同一结构：

```json
{
  "code": "HTTP-422",
  "message": "validation failed on Email (email)",
  "details": [{"field": "Email", "message": "email"}],
  "status": 422,
  "path": "/users",
  "kind": "Validation",
  "requestId": "4f9c..."
}
```

| 字段 | 说明 |
|------|------|
| `code` | 业务错误码；`HttpException.Code` 优先，否则 `HTTP-<status>` |
| `message` | 人类可读文案 |
| `details` | 结构化细节（校验错误列表等） |
| `status` | 与 HTTP 状态码一致 |
| `kind` | 内核错误分类 |
| `requestId` | 启用 `enhancers.RequestID` 后自动回显 |

## 2.2 HttpException

```go
type HttpException struct {
	Status  int
	Message string
	Cause   error
	Headers map[string]string
	Code    string
	Details any
}

func (e *HttpException) Unwrap() error
func (e *HttpException) WithHeaders(map[string]string) *HttpException
func (e *HttpException) WithCode(string) *HttpException
func (e *HttpException) WithDetails(any) *HttpException
func (e *HttpException) WithCause(error) *HttpException
func FromError(err error) (*HttpException, bool)
func StatusFromKind(kind kerrors.Kind) int
```

内置构造器覆盖 400/401/402/403/404/405/406/408/409/410/411/412/413/414/415/416/417/
422/423/424/426/428/429/431/451/500/501/502/503/504/505/507/508/511。
几个带额外参数的：

```go
aop.MethodNotAllowedException("", "GET", "POST")   // 自动写 Allow 响应头
aop.TooManyRequestsException("", 3*time.Second)    // 自动写 Retry-After
```

`FromError` 的识别顺序：`*HttpException`（含包装链）→ `*ValidationException`（422）→
`context.DeadlineExceeded`（504）/ `context.Canceled`（499）→ `KernelError` 分类 → 500。

## 3. 哨兵错误

```go
errors.ErrMethodNotFound
errors.ErrRouteNotFound
errors.ErrInvalidProtocol
errors.ErrBindingNotFound
errors.ErrBindingNotSupported
errors.ErrInvalidHandler
errors.ErrExecutionFailed
errors.ErrGuardRejected
errors.ErrMiddlewareAborted
errors.ErrInvalidParamType
errors.ErrInvalidReturnType
errors.ErrDIResolveFailed
```

用 `errors.Is` 判断，`Normalize` 自动把它们归类到对应 `Kind`：

| 哨兵 | → Kind |
|------|--------|
| `ErrRouteNotFound` | `RouteNotFound` |
| `ErrBindingNotFound` / `ErrBindingNotSupported` / `ErrInvalidParamType` | `Binding` |
| `ErrDIResolveFailed` | `DI` |
| `ErrGuardRejected` | `Forbidden` |
| `ErrExecutionFailed` | `Execution` |
| 其他 | `Unknown` |

## 4. Normalize

```go
func Normalize(protocol, routeKey, moduleKey string, err error) *KernelError
func NormalizeWithMeta(protocol, routeKey, moduleKey string, paramIndex int, meta map[string]any, err error) *KernelError
```

行为：

- `err == nil` → 返回 `nil`
- 已是 `*KernelError` → 只补缺失字段（不覆盖已有值），`meta` 合并写入
- 其他 error → 新建 `KernelError`，`Cause` 保留原错误并做 Kind 分类

## 5. 业务异常：aop.HttpException

```go
type HttpException struct {
	Status  int
	Message string
	Cause   error
}
```

| 构造器 | 状态码 |
|--------|--------|
| `aop.BadRequestException(msg)` | 400 |
| `aop.UnauthorizedException(msg)` | 401 |
| `aop.ForbiddenException(msg)` | 403 |
| `aop.NotFoundException(msg)` | 404 |
| `aop.ConflictException(msg)` | 409 |
| `aop.InternalServerErrorException(msg)` | 500 |

HTTP adapter 遇到 `*aop.HttpException` 时以它的 `Status` 为准（优先于 Kind 映射）。

## 6. 异常过滤器

```go
filter := aop.ExceptionFilterFunc(func(exception interface{}, ctx *aop.ExecutionContext) error {
	err, ok := exception.(error)
	if !ok {
		return nil
	}
	// 自定义日志 / 上报
	return nil   // 返回 nil 表示已处理，不再向上抛
})
```

挂载位置：`UseFilters(...)`（controller / route / gateway / resolver 级）或全局。

## 7. panic 与恢复

- runtime 在 `Execute` 内 recover panic，并归一化为 `KindExecution`
- GraphQL 会把 panic 转成 `errors[]` 条目
- gRPC 会把 panic 转成 `codes.Unknown`（建议配 interceptor 统一映射）
- MQ 收到 error 会 `Nack`，并在 Kafka 场景下不提交 offset

## 8. 排错清单

| 报错 | 定位 |
|------|------|
| `binding resolver not found for type` | 参数类型缺 wrapper，也没注册到 DI |
| `binding not supported for protocol` | 命中了其他协议的 wrapper（如 HTTP 里用 gRPC wrapper） |
| `missing required param "id"` | `required:"true"` 但请求没带 |
| `validation failed on Email (email)` | validator 标签不满足；看 `Meta.errors` |
| `di resolve failed` | Provider 未注册 / 构造返回 error |
| `route not found` | 路由未注册或 path 参数不匹配 |
| `guard rejected access` | Guard 返回 `(false, nil)` 或 error |
| `execution failed` | handler panic，看 `Cause` |
