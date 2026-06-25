// Package metadata 提供框架内部使用的轻量元数据存储。
//
// 它更偏底层基础设施，主要用于在运行期或编译辅助阶段把一些附加信息
// 绑定到函数、方法、对象实例等 target 上，而不是面向业务层的主声明机制。
//
// 在 v3 中，更高层的模块声明、路由声明、binding 声明通常由各协议 DSL、
// registry 和 compiler 负责；`core/metadata` 更像是一层通用的“附着信息仓库”。
package metadata
