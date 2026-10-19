// install.go 说明：OTel 的全局 AOP 已并入模块装配。
//
// otel.Module(cfg) 在 Init 阶段通过 module.Context.Use 贡献 middleware 与 interceptor，
// 用户不再需要调用 Install/InstallOnFactory。
package otel
