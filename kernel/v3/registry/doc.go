// Package registry 提供编译期声明注册表。
//
// 各协议 DSL、integration 或 application 会先把声明写入这里，
// 然后 compiler 再统一读取这些声明，生成 runtime 可安装的产物。
//
// 可以把它理解为 v3 在“声明阶段”的集中仓库。
package registry
