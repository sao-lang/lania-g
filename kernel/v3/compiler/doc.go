// Package compiler 提供协议编译基础设施：将模块树 + registry 声明编译为可安装到 runtime 的产物。
//
// 主要内容：
// - ProtocolPlugin 扩展点：Register/Scan/Compile
// - Compile 入口：组装 BindingRegistry、合并全局 AOP、检测跨协议 routeKey 冲突
// - Install：将编译产物安装到 runtime.Router/Executor
//
// 注意：该包属于框架内部能力，不建议作为应用层的稳定公共 API 直接依赖。
package compiler
