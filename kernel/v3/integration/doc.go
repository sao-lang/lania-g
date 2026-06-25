// Package integration 提供 integration 模块复用的一些底层辅助能力。
//
// 当前这层主要聚焦在“如何把 DI 容器中的对象暴露为 handler 参数 binding”。
// 它本身不是业务 integration，而是给 `integrations/*` 这类上层模块复用的内部工具层。
package integration
