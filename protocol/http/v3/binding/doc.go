// Package http 提供 HTTP 协议下最常用的参数绑定包装类型与默认解析规则。
//
// 对业务开发者来说，这个包通常是最常接触的 binding 包：
// 例如 `Param[T]`、`Query[T]`、`Header[T]`、`Body[T]` 等都定义在这里。
//
// 它们本质上不是业务数据本身，而是告诉 runtime“这个参数该从哪里取值”。
package http
