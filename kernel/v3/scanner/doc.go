// Package scanner 提供模块树到编译快照的扫描能力。
//
// 它的职责比较单一：
// - 从 `module.ModuleRef` 遍历整个模块树
// - 收集 providers、controllers、resolvers 等声明
// - 为 controller / resolver 建立“声明实例 -> 所属模块”的归属索引
//
// 对 compiler 来说，`scanner.Snapshot` 可以理解成“进入协议编译前的统一视图”。
// 各协议插件不需要自己重复遍历模块树，而是直接基于快照做声明筛选、owner 推导和路由编译。
package scanner
