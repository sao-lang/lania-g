// Package graph 提供依赖图诊断工具，用于检测模块 imports 与 provider 依赖的循环等问题。
//
// 主要使用方：
// - module.ModuleLoader.validateGraphs：构建诊断并在发现问题时返回可读的循环信息
//
// 注意：该包属于框架内部能力，不建议作为应用层的稳定公共 API 直接依赖。
package graph
