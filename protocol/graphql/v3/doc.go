// Package graphql 提供 GraphQL 协议适配器、声明 DSL 与编译插件。
//
// 这个包负责把 GraphQL 相关声明写入 registry，
// 再在编译阶段转换为 runtime 可安装的路由与处理器。
//
// 如果你是业务使用者，可以把它看成“GraphQL 的接入入口”；
// 如果你在维护框架，则这里也是 GraphQL 协议编译链路的主要实现位置。
package graphql
