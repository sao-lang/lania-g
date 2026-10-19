---
title: 核心概念
weight: 2
sidebar:
  open: true
toc: false
---

理解这四个概念，就理解了 `lania-g` 的运行方式：

```text
Module  ──►  Registry  ──►  Compiler  ──►  Runtime
（组织代码）  （收集声明）    （生成产物）    （执行请求）
```

- [Module](module)：如何组织模块、导入导出与生命周期
- [依赖注入](di)：容器树、作用域、字段注入与构造函数注入
- [编译与执行](compile-runtime)：声明如何变成可执行的 handler
- [AOP](aop)：五类增强的执行顺序与适用场景
- [Binding](binding)：请求数据如何变成 handler 参数
- [错误与生命周期](errors-lifecycle)：错误模型与启动/关闭钩子
