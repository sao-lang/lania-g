---
title: WebSocket
weight: 4
prev: /docs/protocols/graphql
next: /docs/protocols/mq
---

包：`github.com/sao-lang/lania-g/protocol/ws/v3`

底层使用 `github.com/zishang520/socket.io`（Socket.IO v4 / Engine.IO v4）。

## 1. 创建与挂载

```go
// 共享监听：与 HTTP 同端口，挂载到 /socket.io/
wsAdapter := wsadapter.New()

// 独立监听：自己起一个 HTTP 服务
wsAdapter := wsadapter.New(":3000")

app, _ := application.NewWithOptions(root, opts, httpAdapter, wsAdapter)
app.Listen(":8080")
```

## 2. 声明网关

```go
wsAPI := wsAdapter.API().(*wsadapter.API)

wsAPI.Gateway("/ws/chat", gateway).
	On("users_list", gateway.UsersList).
	On("room_join", gateway.RoomJoin).
	On("room_broadcast", gateway.RoomBroadcast).
	BuildE()
```

- 第一个参数是 **namespace**（不是 URL path 前缀）
- `On(event, handler)` 声明事件

## 3. 连接钩子

```go
type ChatGateway struct{}

func (g *ChatGateway) OnWebSocketConnect(conn any) error      { return nil }
func (g *ChatGateway) OnWebSocketDisconnect(conn any, reason string) {}
func (g *ChatGateway) OnWebSocketError(conn any, err error)   {}
```

## 4. 事件处理器

```go
type roomJoinDTO struct {
	Room string `json:"room" validate:"required"`
}

func (g *ChatGateway) RoomJoin(ctx wsbinding.Context) (any, error) {
	var dto roomJoinDTO
	if err := ctx.ShouldBindMessage(&dto); err != nil {
		return nil, err
	}
	_ = ctx.Join(dto.Room)
	return map[string]any{"id": ctx.ID(), "rooms": ctx.Rooms()}, nil
}
```

也可以使用 `WSMessageBody[T]` 显式声明消息体来源。

## 5. 响应与错误事件

| 结果 | 事件名 |
|------|--------|
| 成功 | `<event>Response` |
| 失败 | `<event>Error` |

可以通过 `WithResponseEventSuffix` / `WithErrorEventSuffix` 改写。

## 6. 房间与广播

```go
func (g *ChatGateway) RoomBroadcast(ctx wsbinding.Context) (any, error) {
	_ = ctx.BroadcastTo("r1", "room_message", map[string]any{"msg": "hello"})
	return map[string]any{"sent": true}, nil
}
```

`wsbinding.Context` 提供：

`Emit` / `Join` / `Leave` / `Rooms` / `BroadcastTo` / `BroadcastToNamespace` /
`RoomLen` / `LeaveAll` / `Disconnect` / `ShouldBindMessage`

{{< callout type="info" >}}
广播能力由 adapter 自身实现，业务不需要持有 socket.io server 实例。
{{< /callout >}}

## 7. 客户端

```bash
npm install socket.io-client@4
```

```js
import { io } from 'socket.io-client';

const socket = io('http://127.0.0.1:8080/ws/chat', {
  extraHeaders: { Authorization: `Bearer ${token}` },
});

socket.emit('users_list', { page: 1, size: 10 });
socket.on('users_listResponse', (data) => console.log(data));
socket.on('users_listError', (err) => console.error(err));
```

{{< callout type="warning" >}}
v3 起不再兼容 `socket.io-client` v2（Engine.IO v3）。
{{< /callout >}}

## 8. 鉴权

WS 的鉴权通常在全局 Interceptor 中完成：

```go
func wsAuthInterceptor(store *MemoryStore) aop.InterceptorFunc {
	return func(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
		if isPublicWSRoute(ctx.RouteKey) {
			return next.Handle()
		}
		hc := ctx.HandlerContext.(*runtime.HandlerContext)
		token, err := extractBearerToken(hc.Request.Headers["Authorization"])
		if err != nil {
			return nil, err
		}
		// 校验 token
		return next.Handle()
	}
}
```
