// types.go 定义 WS 协议暴露给 handler 的 binding wrapper 与辅助类型。
package ws

// WSMessageBody[T] 表示“把当前消息体解成 T”。
// 它是 WS 场景最常用的显式输入 wrapper。
type WSMessageBody[T any] struct{ Value T }

// Header[T] 表示按 key 读取请求头。
// 在 WS 场景里，这些 header 通常来自握手阶段，而不是每条消息自带的 header。
type Header[T any] struct{ Value T }
// WSHeaders 是 Header 的别名（兼容旧命名）。
type WSHeaders[T any] = Header[T]

// 下面这组 wrapper 主要暴露 socket 连接现场元信息，而不是消息体本身。
// 它们通常由 adapter 在回调入口直接写入 metadata。

// WSConnectedSocket 表示当前连接的 socket 对象（由 adapter 注入）。
type WSConnectedSocket struct{ Value any }
// WSEvent 表示当前触发的事件名。
type WSEvent struct{ Value string }
// WSSocketID 表示当前连接的 socket ID。
type WSSocketID struct{ Value string }
// WSRooms 表示当前连接加入的房间列表。
type WSRooms struct{ Value []string }
// Headers 表示所有 header 的多值映射快照。
type Headers map[string][]string
