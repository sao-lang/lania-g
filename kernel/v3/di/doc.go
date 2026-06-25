// Package di 提供 v3 的依赖注入容器与 provider 模型。
//
// 它是模块系统的基础设施之一，负责：
// - 注册 provider / value / factory
// - 按 token 解析实例
// - 管理 singleton、request 等作用域
//
// 对业务开发者来说，通常通过 `module` 包间接使用它；
// 对框架维护者来说，这里是实例装配与注入行为的核心基础层。
package di
