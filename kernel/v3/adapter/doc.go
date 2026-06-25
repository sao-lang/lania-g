// Package adapter 定义协议适配器与宿主之间的通用接口。
//
// 可以把它理解为 `application` 与各协议 adapter 之间的“最小契约层”：
// - `Host` 代表 adapter 运行时依赖的宿主能力
// - `Adapter` 代表一个可挂载、可启动、可停止的协议适配器
// - `SharedListenerConfigurator` 用于支持 `app.Listen(addr)` 这类共享监听地址场景
//
// 业务代码通常不直接实现这里的接口，但在理解框架扩展方式时，
// 这个包是 adapter 体系最核心的边界之一。
package adapter
