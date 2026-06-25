// Package factory 提供更偏“装配脚手架”的应用构建入口。
//
// 与直接调用 `application.NewWithOptions(...)` 或 `application.NewCompat(...)` 相比，
// 这层更适合用链式方式逐步组装：
// - root modules
// - adapters
// - 全局 AOP
//
// 最终它仍然会把这些输入转换成 `application.Application`，
// 因此可以把它理解为一层对 application 的便捷封装。
package factory
