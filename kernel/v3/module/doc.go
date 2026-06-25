// Package module 提供 v3 的模块模型、生命周期接口与模块装配工具。
//
// 可以把模块理解为“框架中的组织单元”：
// - `Imports` 描述依赖哪些模块
// - `Providers` 描述可注入的依赖
// - `Controllers/Resolvers` 描述对外暴露的协议处理入口
// - `Exports` 描述哪些能力可被其他模块复用
//
// 对大多数业务代码来说，`CreateModule(...)` 是最常见的声明入口。
package module
