// hooks.go 提供 WS adapter 的连接与事件生命周期钩子定义。
package ws

// 这组三个 hook 接口都是“可选能力”：
// gateway 实现了才会被 adapter 调用，没实现就静默跳过。

// OnWebSocketConnect 可由 gateway 实现，用于处理客户端连接事件。
type OnWebSocketConnect interface {
	OnWebSocketConnect(conn any) error
}

// OnWebSocketDisconnect 可由 gateway 实现，用于处理客户端断开连接事件。
type OnWebSocketDisconnect interface {
	OnWebSocketDisconnect(conn any, reason string)
}

// OnWebSocketError 可由 gateway 实现，用于处理 socket.io 连接上的错误事件。
type OnWebSocketError interface {
	OnWebSocketError(conn any, err error)
}
