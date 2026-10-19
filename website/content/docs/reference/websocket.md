---
title: WebSocket
weight: 9
prev: /docs/reference/graphql
next: /docs/reference/mq
---

包：`github.com/sao-lang/lania-g/protocol/ws/v3`

底层：`github.com/zishang520/socket.io`（**Socket.IO v4 / Engine.IO v4**）。

## 1. Adapter

| 方法 | 说明 |
|------|------|
| `New()` | 共享监听，挂到 HTTP 适配器的 `/socket.io/` |
| `New(":3000")` | 独立监听 |
| `WithAddr(addr)` | 运行时改地址 |
| `AsStandalone(addr)` | 转为独立监听 |
| `WithSocketPath(path)` | 改 socket 路径 |
| `WithResponseEventSuffix(s)` | 成功事件后缀（默认 `Response`） |
| `WithErrorEventSuffix(s)` | 错误事件后缀（默认 `Error`） |
| `Handler()` | 取出 `http.Handler` |
| `API()` | 取出 DSL 上下文 |
| `BroadcastToRoom(ns, room, event, args...)` | 房间广播 |
| `BroadcastToNamespace(ns, event, args...)` | 命名空间广播 |
| `RoomLen(ns, room)` | 房间人数 |

```go
httpAdp := httpadapter.New()
wsAdp := wsadapter.New()   // 共享监听

app, _ := application.NewWithOptions(root, application.Options{
	Registry: registry.New(),
}, httpAdp, wsAdp)
app.Listen(":8080")
```

## 2. 网关 DSL

```go
wsAPI := wsAdp.API().(*wsadapter.API)

wsAPI.Gateway("/ws/chat", gateway).      // 第一个参数是 namespace
	On("users_list", gateway.UsersList).
	On("room_join", gateway.RoomJoin).
	On("room_broadcast", gateway.RoomBroadcast).
	BuildE()
```

| Builder | 方法 |
|---------|------|
| `API` | `Gateway(namespace, receiver)` |
| `GatewayBuilder` | `On(event, handler)` |
| | `UseMiddlewares/UseGuards/UseInterceptors/UsePipes/UseFilters` |
| | `UseParamPipes(index, ...)` |
| | `Build()/BuildE()/Err()` |
| `HandlerBuilder` | 事件级 AOP：`UseGuards/UseInterceptors/UsePipes/UseFilters` |

> Gateway 必须放进模块的 `controllers` 槽位（`module.CreateModule(nil, providers, []any{gw}, nil, nil)`），否则 DI 拿不到 receiver。

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
