---
title: WebSocket
weight: 9
prev: /docs/reference/graphql
next: /docs/reference/mq
---

包：`github.com/sao-lang/lania-g/protocol/ws/v3`

底层：`github.com/zishang520/socket.io`（**Socket.IO v4 / Engine.IO v4**）。

## 1. 模块入口

```go
func Module(cfg Config) module.Module
```

| Config 字段 | 说明 |
|-------------|------|
| `Addr` | 监听地址，例如 `":8081"` |

```go
root := module.New(module.Metadata{
	Imports:     []module.Module{ws.Module(ws.Config{Addr: ":8081"})},
	Controllers: []any{&ChatGateway{}},
})
```

底层为 `github.com/zishang520/socket.io`（**Socket.IO v4 / Engine.IO v4**）。

## 2. 网关声明

```go
type Controller interface {
	Declare(*Routes)
}
```

```go
func (g *ChatGateway) Declare(r *ws.Routes) {
	r.Gateway("/ws/chat"). // namespace
		On("users_list", g.UsersList).
		On("room_join", g.RoomJoin).
		On("room_broadcast", g.RoomBroadcast)
}
```

| 类型 | 方法 |
|------|------|
| `Routes` | `Gateway(namespace) *GatewayBuilder` |
| `GatewayBuilder` | `On(event, handler)` |
| | `UseMiddlewares/UseGuards/UseInterceptors/UsePipes/UseFilters` |
| | `UseParamPipes(index, ...)` |
| | `Group(...)` / `Err()` |
| `HandlerBuilder` | 事件级 AOP：`UseGuards/UseInterceptors/UsePipes/UseFilters` |

{{< callout type="info" >}}
Gateway 必须放进模块的 `Controllers` 槽位；声明阶段由模块树收集，
不需要 `Build()`，也不需要手动拿 `API()`。
{{< /callout >}}

## 2.1 房间与广播

| 方法 | 说明 |
|------|------|
| `BroadcastToRoom(ns, room, event, args...)` | 房间广播 |
| `BroadcastToNamespace(ns, event, args...)` | 命名空间广播 |
| `RoomLen(ns, room)` | 房间人数 |

## 3. 连接钩子

```go
func (g *ChatGateway) OnWebSocketConnect(conn any) error            { return nil }
func (g *ChatGateway) OnWebSocketDisconnect(conn any, reason string) {}
func (g *ChatGateway) OnWebSocketError(conn any, err error)          {}
```

钩子可选，签名匹配才会被调用。

## 4. 事件处理器

两种签名风格：

```go
// 风格一：显式 body wrapper
func (g *ChatGateway) UsersCreate(body wsbinding.WSMessageBody[createDTO]) (any, error) {
	return store.create(body.Value)
}

// 风格二：用 Context 手动绑定
func (g *ChatGateway) RoomJoin(ctx wsbinding.Context) (any, error) {
	var dto roomJoinDTO
	if err := ctx.ShouldBindMessage(&dto); err != nil {
		return nil, err
	}
	_ = ctx.Join(dto.Room)
	return map[string]any{"id": ctx.ID(), "rooms": ctx.Rooms()}, nil
}
```

## 5. 上下文能力

`wsbinding.Context` 方法：

| 分类 | 方法 |
|------|------|
| 元信息 | `Event()` `Namespace()` `ID()` `URL()` `RemoteAddr()` `Query(key)` `Request()` `Headers()` |
| 数据 | `Message()` `ShouldBindMessage(obj)` |
| 连接 | `Conn()` `Server()` `Disconnect()` `LeaveAll()` |
| 房间 | `Join(room)` `Leave(room)` `Rooms()` `RoomLen(room)` |
| 发送 | `Emit(event, args...)` |
| 广播 | `BroadcastTo(room, event, args...)` `BroadcastToNamespace(event, args...)` |
| 底层 | `HandlerContext() *runtime.HandlerContext` |

## 6. 请求绑定

| wrapper | 数据源 |
|---------|--------|
| `WSMessageBody[T]` | 事件消息体 |
| `Context` | `WsContext` |
| `Header[T]` / `WSHeaders[T]` | 握手 header |
| `Headers` | 全部 header |
| `WSConnectedSocket` | 连接对象 |
| `WSEvent` | 事件名 |
| `WSSocketID` | socket id |
| `WSRooms` | 当前房间列表 |

`ShouldBindMessage` 会走 validator（需 `WithValidatorV10()`），错误归一化为 `KindValidation`。

## 7. 响应与错误事件

| 结果 | 事件名 |
|------|--------|
| 成功 | `<event>Response` |
| 失败 | `<event>Error` |

payload 是 handler 的返回值 / 归一化后的错误对象。

```go
wsAdp.WithResponseEventSuffix("Result").WithErrorEventSuffix("Failed")
```

## 8. 客户端

```bash
npm install socket.io-client@4
```

```js
import { io } from 'socket.io-client';

const socket = io('http://127.0.0.1:8080/ws/chat', {
  extraHeaders: { Authorization: 'Bearer <token>' },
});

socket.emit('users_list', { page: 1, size: 10 });
socket.on('users_listResponse', (data) => console.log(data));
socket.on('users_listError', (err) => console.error(err));

// 房间
socket.emit('room_join', { room: 'r1' });
socket.on('room_message', (msg) => console.log(msg));
```

{{< callout type="warning" >}}
从 v3 起只支持 Socket.IO v4 客户端，v2（Engine.IO v3）客户端握手后会立即断开。
{{< /callout >}}

## 9. 鉴权

WS 没有 HTTP middleware，鉴权走全局 Interceptor：

```go
func wsAuthInterceptor(store *MemoryStore) aop.InterceptorFunc {
	return func(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
		if isPublicWSEvent(ctx.RouteKey) {
			return next.Handle()
		}
		hc := ctx.HandlerContext.(*runtime.HandlerContext)
		token, err := extractBearerToken(hc.Request.Headers["Authorization"])
		if err != nil {
			return nil, err
		}
		if _, err := store.verify(token); err != nil {
			return nil, err
		}
		return next.Handle()
	}
}

app.UseGlobalInterceptors(wsAuthInterceptor(store))
```

## 10. 排错

| 现象 | 原因 |
|------|------|
| 握手成功立刻断开 | 客户端是 Socket.IO v2 |
| `unable to find handler for event` | `On` 未声明该事件，或 gateway 未放入 controllers 槽位 |
| 客户端收不到响应 | 事件监听名少了 `Response` 后缀 |
| 房间广播无人收到 | namespace 与 `ctx.Namespace()` 不一致 |
