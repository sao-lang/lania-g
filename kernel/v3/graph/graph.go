// graph.go 提供模块图与 provider 图的循环依赖诊断能力。
//
// 它不参与真实依赖解析，而是给 module loader 提供“启动前图校验”的结构化工具。
package graph

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/di"
)

// Diagnostics 汇总模块图和 provider 图的循环依赖诊断结果。
// 当前主要关注循环依赖；后续若扩展其他图问题，也会优先落在这里。
type Diagnostics struct {
	ModuleCycles   [][]string
	ProviderCycles [][]string
}

// HasIssues 用于快速判断是否存在循环依赖等问题。
func (d *Diagnostics) HasIssues() bool {
	return d != nil && (len(d.ModuleCycles) > 0 || len(d.ProviderCycles) > 0)
}

// String 将诊断结果格式化为可读字符串，便于错误消息/日志输出。
func (d *Diagnostics) String() string {
	if d == nil || !d.HasIssues() {
		return ""
	}
	var parts []string
	for _, cycle := range d.ModuleCycles {
		parts = append(parts, "module cycle: "+strings.Join(cycle, " -> "))
	}
	for _, cycle := range d.ProviderCycles {
		parts = append(parts, "provider cycle: "+strings.Join(cycle, " -> "))
	}
	return strings.Join(parts, "; ")
}

// ModuleGraph 表示模块 imports 之间的依赖图。
type ModuleGraph struct {
	nodes map[reflect.Type]struct{}
	edges map[reflect.Type][]reflect.Type
}

// ModuleNodeInput 是构建 ModuleGraph 时使用的输入结构。
type ModuleNodeInput struct {
	Type    reflect.Type
	Imports []reflect.Type
}

// NewModuleGraph 根据模块节点输入创建模块依赖图（imports 边）。
func NewModuleGraph(mods []ModuleNodeInput) *ModuleGraph {
	g := &ModuleGraph{
		nodes: make(map[reflect.Type]struct{}),
		edges: make(map[reflect.Type][]reflect.Type),
	}
	for _, mod := range mods {
		if mod.Type == nil {
			continue
		}
		g.nodes[mod.Type] = struct{}{}
		for _, imp := range mod.Imports {
			if imp == nil {
				continue
			}
			g.edges[mod.Type] = appendUniqueType(g.edges[mod.Type], imp)
		}
	}
	return g
}

// DetectCycles 检测模块 imports 是否存在环，返回所有检测到的环（去重后）。
// 这里保留“所有命中的环”，比单个报错更适合启动诊断输出。
func (g *ModuleGraph) DetectCycles() [][]reflect.Type {
	visited := make(map[reflect.Type]bool)
	stack := make(map[reflect.Type]bool)
	var path []reflect.Type
	var cycles [][]reflect.Type

	var dfs func(node reflect.Type)
	dfs = func(node reflect.Type) {
		visited[node] = true
		stack[node] = true
		path = append(path, node)
		for _, next := range g.edges[node] {
			if !visited[next] {
				dfs(next)
				continue
			}
			if stack[next] {
				cycles = append(cycles, extractTypeCycle(path, next))
			}
		}
		stack[node] = false
		path = path[:len(path)-1]
	}

	keys := sortedTypeKeys(g.nodes)
	for _, key := range keys {
		if !visited[key] {
			dfs(key)
		}
	}
	return dedupeTypeCycles(cycles)
}

// TopologicalSort 对模块图做拓扑排序；若存在环则返回错误。
func (g *ModuleGraph) TopologicalSort() ([]reflect.Type, error) {
	return topoSortTypes(g.nodes, g.edges)
}

// ProviderGraph 表示 provider 之间的依赖图。
type ProviderGraph struct {
	nodes map[string]*di.Provider
	edges map[string][]string
}

// NewProviderGraph 根据 providers 构建 provider 依赖图。
//
// 依赖来源：
// - provider.Deps 显式声明优先
// - Existing provider 依赖 UseExisting
// - Class provider 默认把“导出字段类型”视为依赖（作为一种近似诊断）
func NewProviderGraph(providers map[interface{}]*di.Provider) *ProviderGraph {
	g := &ProviderGraph{
		nodes: make(map[string]*di.Provider),
		edges: make(map[string][]string),
	}
	for token, provider := range providers {
		key := tokenKey(token)
		g.nodes[key] = provider
		deps := providerDependencies(provider)
		for _, dep := range deps {
			g.edges[key] = appendUniqueString(g.edges[key], tokenKey(dep))
		}
	}
	return g
}

// DetectCycles 检测 provider 依赖是否存在环，返回所有检测到的环（去重后）。
func (g *ProviderGraph) DetectCycles() [][]string {
	visited := make(map[string]bool)
	stack := make(map[string]bool)
	var path []string
	var cycles [][]string

	var dfs func(node string)
	dfs = func(node string) {
		visited[node] = true
		stack[node] = true
		path = append(path, node)
		for _, next := range g.edges[node] {
			if !visited[next] {
				dfs(next)
				continue
			}
			if stack[next] {
				cycles = append(cycles, extractStringCycle(path, next))
			}
		}
		stack[node] = false
		path = path[:len(path)-1]
	}

	keys := sortedStringKeys(g.nodes)
	for _, key := range keys {
		if !visited[key] {
			dfs(key)
		}
	}
	return dedupeStringCycles(cycles)
}

// TopologicalSort 对 provider 图做拓扑排序；若存在环则返回错误。
func (g *ProviderGraph) TopologicalSort() ([]string, error) {
	return topoSortStrings(g.nodes, g.edges)
}

// BuildDiagnostics 是 module loader 的统一入口：同时检测 module imports 与 provider 依赖的循环。
// 返回 nil 表示没有问题；否则返回包含 cycle 列表的 Diagnostics。
func BuildDiagnostics(mods []ModuleNodeInput, providers map[interface{}]*di.Provider) *Diagnostics {
	d := &Diagnostics{}
	for _, cycle := range NewModuleGraph(mods).DetectCycles() {
		d.ModuleCycles = append(d.ModuleCycles, stringifyTypes(cycle))
	}
	d.ProviderCycles = append(d.ProviderCycles, NewProviderGraph(providers).DetectCycles()...)
	if !d.HasIssues() {
		return nil
	}
	return d
}

// tokenKey 将任意 token 规范化为稳定字符串 key，用于 provider 图节点标识。
func tokenKey(token interface{}) string {
	if token == nil {
		return "<nil>"
	}
	if t, ok := token.(reflect.Type); ok {
		return t.String()
	}
	return fmt.Sprintf("%T:%v", token, token)
}

// providerDependencies 推导一个 provider 的依赖 token 列表（用于诊断，不用于真正注入）。
// 它是“近似诊断模型”，不是运行时真实注入算法的镜像。
func providerDependencies(provider *di.Provider) []interface{} {
	if provider == nil {
		return nil
	}
	if len(provider.Deps) > 0 {
		return provider.Deps
	}
	if provider.Type == di.ProviderTypeExisting && provider.UseExisting != nil {
		return []interface{}{provider.UseExisting}
	}
	if provider.Type == di.ProviderTypeClass && provider.UseClass != nil {
		var deps []interface{}
		typ := provider.UseClass
		if typ.Kind() == reflect.Ptr {
			typ = typ.Elem()
		}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if !field.IsExported() {
				continue
			}
			// 这里把导出字段类型当成潜在依赖，只是为了给“无显式 Deps”的 class provider
			// 提供一个可用的循环依赖近似诊断。
			deps = append(deps, field.Type)
		}
		return deps
	}
	return nil
}

// appendUniqueType 追加一个 reflect.Type 到切片中（若已存在则不追加）。
func appendUniqueType(items []reflect.Type, value reflect.Type) []reflect.Type {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

// appendUniqueString 追加一个 string 到切片中（若已存在则不追加）。
func appendUniqueString(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

// sortedTypeKeys 返回按 Type.String() 排序后的 reflect.Type keys（用于稳定遍历顺序）。
func sortedTypeKeys[T any](nodes map[reflect.Type]T) []reflect.Type {
	keys := make([]reflect.Type, 0, len(nodes))
	for key := range nodes {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].String() < keys[j].String() })
	return keys
}

// sortedStringKeys 返回按字典序排序后的 string keys（用于稳定遍历顺序）。
func sortedStringKeys[T any](nodes map[string]T) []string {
	keys := make([]string, 0, len(nodes))
	for key := range nodes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// extractTypeCycle 从 DFS path 中截取以 start 为起点的环，并在末尾追加 start 形成闭环展示。
func extractTypeCycle(path []reflect.Type, start reflect.Type) []reflect.Type {
	var cycle []reflect.Type
	found := false
	for _, item := range path {
		if item == start {
			found = true
		}
		if found {
			cycle = append(cycle, item)
		}
	}
	return append(cycle, start)
}

// extractStringCycle 从 DFS path 中截取以 start 为起点的环，并在末尾追加 start 形成闭环展示。
func extractStringCycle(path []string, start string) []string {
	var cycle []string
	found := false
	for _, item := range path {
		if item == start {
			found = true
		}
		if found {
			cycle = append(cycle, item)
		}
	}
	return append(cycle, start)
}

// dedupeTypeCycles 对类型环进行去重（用 stringify 后 join 得到的 key）。
func dedupeTypeCycles(cycles [][]reflect.Type) [][]reflect.Type {
	seen := make(map[string]bool)
	var out [][]reflect.Type
	for _, cycle := range cycles {
		key := strings.Join(stringifyTypes(cycle), "|")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, cycle)
	}
	return out
}

// dedupeStringCycles 对字符串环进行去重（用 join 得到的 key）。
func dedupeStringCycles(cycles [][]string) [][]string {
	seen := make(map[string]bool)
	var out [][]string
	for _, cycle := range cycles {
		key := strings.Join(cycle, "|")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, cycle)
	}
	return out
}

// stringifyTypes 将 reflect.Type 列表转为 string 列表（使用 Type.String()）。
func stringifyTypes(items []reflect.Type) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.String())
	}
	return out
}

// topoSortTypes 对类型节点图做拓扑排序；若发现回边则返回错误。
func topoSortTypes[T any](nodes map[reflect.Type]T, edges map[reflect.Type][]reflect.Type) ([]reflect.Type, error) {
	visited := make(map[reflect.Type]bool)
	temp := make(map[reflect.Type]bool)
	var result []reflect.Type
	var visit func(reflect.Type) error
	visit = func(node reflect.Type) error {
		if temp[node] {
			return fmt.Errorf("circular module dependency detected at %s", node.String())
		}
		if visited[node] {
			return nil
		}
		temp[node] = true
		for _, next := range edges[node] {
			if err := visit(next); err != nil {
				return err
			}
		}
		temp[node] = false
		visited[node] = true
		result = append(result, node)
		return nil
	}
	for _, node := range sortedTypeKeys(nodes) {
		if !visited[node] {
			if err := visit(node); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

// topoSortStrings 对字符串节点图做拓扑排序；若发现回边则返回错误。
func topoSortStrings[T any](nodes map[string]T, edges map[string][]string) ([]string, error) {
	visited := make(map[string]bool)
	temp := make(map[string]bool)
	var result []string
	var visit func(string) error
	visit = func(node string) error {
		if temp[node] {
			return fmt.Errorf("circular provider dependency detected at %s", node)
		}
		if visited[node] {
			return nil
		}
		temp[node] = true
		for _, next := range edges[node] {
			if _, ok := nodes[next]; !ok {
				continue
			}
			if err := visit(next); err != nil {
				return err
			}
		}
		temp[node] = false
		visited[node] = true
		result = append(result, node)
		return nil
	}
	for _, node := range sortedStringKeys(nodes) {
		if !visited[node] {
			if err := visit(node); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}
