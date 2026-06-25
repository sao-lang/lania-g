// diagnostics.go 定义编译期诊断模型，以及错误冻结/冲突包装辅助。
//
// 这一层的核心目标不是参与编译逻辑本身，
// 而是把“编译过程中知道的信息”稳定地保留下来，供启动前诊断、快照测试和排障输出使用。
package compiler

import (
	"errors"
	"fmt"
	"strings"

	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// AOPDiagnostics 汇总全局 AOP 声明数量。
type AOPDiagnostics struct {
	Middlewares  int `json:"middlewares"`
	Guards       int `json:"guards"`
	Interceptors int `json:"interceptors"`
	Pipes        int `json:"pipes"`
	Filters      int `json:"filters"`
}

// ModuleRouteDiagnostics 描述某个协议下，一个模块拥有的已编译路由数量。
type ModuleRouteDiagnostics struct {
	ModuleKey string   `json:"moduleKey"`
	Routes    int      `json:"routes"`
	RouteKeys []string `json:"routeKeys"`
}

// RuntimeFallbackDiagnostics 描述当前仍保留的 runtime fallback 场景。
type RuntimeFallbackDiagnostics struct {
	Key      string `json:"key"`
	Category string `json:"category"`
	Summary  string `json:"summary"`
}

// RegistryFallbackDiagnostics 描述当前仍保留的 registry fallback 场景。
type RegistryFallbackDiagnostics struct {
	Key      string `json:"key"`
	Category string `json:"category"`
	Summary  string `json:"summary"`
}

// CompatFallbackCategorySummary 描述一类 compat 写来源的聚合结果。
type CompatFallbackCategorySummary struct {
	Category string `json:"category"`
	Hits     int    `json:"hits"`
	Sources  int    `json:"sources"`
}

// CompatFallbackSourceSummary 描述某个 compat 写来源的命中次数。
type CompatFallbackSourceSummary struct {
	Source string `json:"source"`
	Hits   int    `json:"hits"`
}

// ProtocolDiagnostics 描述某个协议插件在本次编译中的统计信息。
// 它更偏“本次编译发生了什么”，而不是长期运行时指标。
type ProtocolDiagnostics struct {
	Protocol         runtime.Protocol         `json:"protocol"`
	PluginID         string                   `json:"pluginId"`
	DeclarationKinds map[string]int           `json:"declarationKinds"`
	DeclarationCount int                      `json:"declarationCount"`
	RouteCount       int                      `json:"routeCount"`
	RouteContainers  int                      `json:"routeContainers"`
	OwnerModules     []ModuleRouteDiagnostics `json:"ownerModules"`
}

// RouteConflict 记录一次跨协议或跨插件 routeKey 冲突。
type RouteConflict struct {
	RouteKey       string           `json:"routeKey"`
	Protocol       runtime.Protocol `json:"protocol"`
	PluginID       string           `json:"pluginId"`
	ExistingRoute  string           `json:"existingRoute"`
	ExistingPlugin string           `json:"existingPlugin"`
	Reason         string           `json:"reason"`
}

// CompileDiagnostics 汇总一次完整编译过程的诊断信息。
//
// 它既可用于启动前预检查，也可在编译失败时附着到 CompileError 上帮助排障。
//
// JSON 合同说明：
// - 该结构及其直接嵌套摘要对象会被外部工具、测试快照和诊断输出直接消费。
// - 对外 JSON 键名固定使用 lowerCamelCase。
// - 若未来继续扩展，优先遵循“只增不删、不改已有键名”。
type CompileDiagnostics struct {
	RegisteredPlugins        []string                                  `json:"registeredPlugins"`
	ProtocolOrder            []runtime.Protocol                        `json:"protocolOrder"`
	DeclarationCounts        map[string]map[string]int                 `json:"declarationCounts"`
	BindingResolverCount     int                                       `json:"bindingResolverCount"`
	RegistrySource           string                                    `json:"registrySource"`
	GlobalAOP                AOPDiagnostics                            `json:"globalAOP"`
	Protocols                map[runtime.Protocol]*ProtocolDiagnostics `json:"protocols"`
	RegistryFallbacks        []RegistryFallbackDiagnostics             `json:"registryFallbacks"`
	RuntimeFallbacks         []RuntimeFallbackDiagnostics              `json:"runtimeFallbacks"`
	CompatFallbackCategories []CompatFallbackCategorySummary           `json:"compatFallbackCategories"`
	CompatFallbackSources    []CompatFallbackSourceSummary             `json:"compatFallbackSources"`
	RouteConflicts           []RouteConflict                           `json:"routeConflicts"`
	Errors                   []string                                  `json:"errors"`
	Warnings                 []string                                  `json:"warnings"`
}

// HasCompatFallbacks 返回当前诊断中是否存在 compat fallback 聚合结果。
func (d *CompileDiagnostics) HasCompatFallbacks() bool {
	return d != nil && (len(d.CompatFallbackCategories) > 0 || len(d.CompatFallbackSources) > 0)
}

// Clone 返回 CompileDiagnostics 的深拷贝快照。
//
// 用途：在返回错误时把当时的诊断信息冻结下来，避免后续编译过程继续修改同一对象导致信息漂移。
func (d *CompileDiagnostics) Clone() *CompileDiagnostics {
	return d.clone()
}

// clone 是 Clone 的内部实现。
func (d *CompileDiagnostics) clone() *CompileDiagnostics {
	if d == nil {
		return nil
	}
	out := &CompileDiagnostics{
		RegisteredPlugins:        append([]string{}, d.RegisteredPlugins...),
		ProtocolOrder:            append([]runtime.Protocol{}, d.ProtocolOrder...),
		DeclarationCounts:        cloneDeclarationCounts(d.DeclarationCounts),
		BindingResolverCount:     d.BindingResolverCount,
		RegistrySource:           d.RegistrySource,
		GlobalAOP:                d.GlobalAOP,
		Protocols:                make(map[runtime.Protocol]*ProtocolDiagnostics, len(d.Protocols)),
		RegistryFallbacks:        cloneRegistryFallbackDiagnostics(d.RegistryFallbacks),
		RuntimeFallbacks:         cloneRuntimeFallbackDiagnostics(d.RuntimeFallbacks),
		CompatFallbackCategories: cloneCompatFallbackCategorySummaries(d.CompatFallbackCategories),
		CompatFallbackSources:    cloneCompatFallbackSourceSummaries(d.CompatFallbackSources),
		RouteConflicts:           append([]RouteConflict{}, d.RouteConflicts...),
		Errors:                   append([]string{}, d.Errors...),
		Warnings:                 append([]string{}, d.Warnings...),
	}
	for protocol, pd := range d.Protocols {
		if pd == nil {
			continue
		}
		copyPD := *pd
		copyPD.DeclarationKinds = cloneDeclarationCountMap(pd.DeclarationKinds)
		copyPD.OwnerModules = cloneModuleRouteDiagnostics(pd.OwnerModules)
		out.Protocols[protocol] = &copyPD
	}
	return out
}

func cloneModuleRouteDiagnostics(src []ModuleRouteDiagnostics) []ModuleRouteDiagnostics {
	if len(src) == 0 {
		return nil
	}
	out := make([]ModuleRouteDiagnostics, 0, len(src))
	for _, item := range src {
		copyItem := item
		copyItem.RouteKeys = append([]string{}, item.RouteKeys...)
		out = append(out, copyItem)
	}
	return out
}

func cloneRuntimeFallbackDiagnostics(src []RuntimeFallbackDiagnostics) []RuntimeFallbackDiagnostics {
	if len(src) == 0 {
		return nil
	}
	return append([]RuntimeFallbackDiagnostics{}, src...)
}

func cloneRegistryFallbackDiagnostics(src []RegistryFallbackDiagnostics) []RegistryFallbackDiagnostics {
	if len(src) == 0 {
		return nil
	}
	return append([]RegistryFallbackDiagnostics{}, src...)
}

func cloneCompatFallbackCategorySummaries(src []CompatFallbackCategorySummary) []CompatFallbackCategorySummary {
	if len(src) == 0 {
		return nil
	}
	return append([]CompatFallbackCategorySummary{}, src...)
}

func cloneCompatFallbackSourceSummaries(src []CompatFallbackSourceSummary) []CompatFallbackSourceSummary {
	if len(src) == 0 {
		return nil
	}
	return append([]CompatFallbackSourceSummary{}, src...)
}

// cloneDeclarationCounts 深拷贝 pluginID -> kind -> count 的统计信息。
func cloneDeclarationCounts(src map[string]map[string]int) map[string]map[string]int {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]map[string]int, len(src))
	for pluginID, kinds := range src {
		out[pluginID] = cloneDeclarationCountMap(kinds)
	}
	return out
}

// cloneDeclarationCountMap 深拷贝 kind -> count map。
func cloneDeclarationCountMap(src map[string]int) map[string]int {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]int, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// countDecls 汇总一个 plugin 的所有 kind 声明数量。
func countDecls(kinds map[string]int) int {
	total := 0
	for _, count := range kinds {
		total += count
	}
	return total
}

// recordCompileError 将编译错误写入 diagnostics，并返回一个带 diagnostics 快照的 CompileError。
//
// 注意：如果 err 包含 RouteConflictError，会把冲突信息也追加到 diag.RouteConflicts。
func recordCompileError(diag *CompileDiagnostics, err error) error {
	if diag != nil && err != nil {
		var routeConflictErr *RouteConflictError
		if errors.As(err, &routeConflictErr) {
			diag.RouteConflicts = append(diag.RouteConflicts, routeConflictErr.Conflict)
		}
		diag.Errors = append(diag.Errors, err.Error())
	}
	if err == nil {
		return nil
	}
	return &CompileError{
		Cause:       err,
		Diagnostics: diag.clone(),
	}
}

// CompileError 表示编译阶段的错误，并携带当时的诊断信息快照。
// 外层应用通常直接看到这个错误，而不是原始 Cause。
type CompileError struct {
	Cause       error
	Diagnostics *CompileDiagnostics
}

// Error 实现 error 接口，返回底层原因错误消息。
func (e *CompileError) Error() string {
	if e == nil || e.Cause == nil {
		return "compile failed"
	}
	base := e.Cause.Error()
	if summary := ownerDiagnosticSummary(e.Cause); summary != "" {
		return base + "\nowner diagnostics: " + summary
	}
	return base
}

// Unwrap 返回底层原因错误，使 errors.Is / errors.As 能穿透 CompileError。
func (e *CompileError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ownerDiagnosticSummary 把 owner 解析失败场景格式化成更可读的一行摘要。
// 它主要服务编译失败时的终端/日志输出，不改变结构化 Meta 本身。
func ownerDiagnosticSummary(err error) string {
	var kernelErr *kerrors.KernelError
	if !errors.As(err, &kernelErr) || kernelErr == nil {
		return ""
	}
	if kernelErr.Meta == nil {
		return ""
	}
	ownerKind, _ := kernelErr.Meta["ownerKind"].(string)
	ownerStatus, _ := kernelErr.Meta["ownerStatus"].(string)
	ownerToken, _ := kernelErr.Meta["ownerToken"].(string)
	candidates, _ := kernelErr.Meta["ownerCandidates"].([]string)
	if ownerKind == "" || ownerStatus == "" {
		return ""
	}
	switch ownerStatus {
	case "ambiguous":
		if len(candidates) == 0 {
			return fmt.Sprintf("%s token %s matches multiple module owners", ownerKind, defaultOwnerToken(ownerToken))
		}
		return fmt.Sprintf("%s token %s matches multiple module owners (%s)", ownerKind, defaultOwnerToken(ownerToken), strings.Join(candidates, ", "))
	case "missing":
		return fmt.Sprintf("%s token %s is not attached to any module owner", ownerKind, defaultOwnerToken(ownerToken))
	case "nil_target":
		return fmt.Sprintf("%s target is nil before module owner resolution", ownerKind)
	default:
		return fmt.Sprintf("%s owner resolution failed (status=%s token=%s)", ownerKind, ownerStatus, defaultOwnerToken(ownerToken))
	}
}

func defaultOwnerToken(token string) string {
	if strings.TrimSpace(token) == "" {
		return "<unknown>"
	}
	return token
}

// RouteConflictError 表示在编译阶段发现了 routeKey 冲突。
type RouteConflictError struct {
	Conflict RouteConflict
	Cause    error
}

// Error 实现 error 接口，优先返回底层 Cause 的错误消息。
func (e *RouteConflictError) Error() string {
	if e == nil || e.Cause == nil {
		return "route conflict detected"
	}
	return e.Cause.Error()
}

// Unwrap 返回底层原因错误，使 errors.Is / errors.As 能穿透 RouteConflictError。
func (e *RouteConflictError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// NewRouteConflictError 构造一个路由冲突错误，并将其 Cause 包装为 KernelError。
//
// 该错误通常在 Compile 阶段发现“跨协议/跨插件 routeKey 冲突”时返回。
func NewRouteConflictError(conflict RouteConflict) error {
	return &RouteConflictError{
		Conflict: conflict,
		Cause: &kerrors.KernelError{
			Kind:     kerrors.KindExecution,
			Protocol: string(conflict.Protocol),
			RouteKey: conflict.RouteKey,
			Message:  fmt.Sprintf("route conflict detected for %s (%s)", conflict.RouteKey, conflict.Reason),
			Meta: map[string]interface{}{
				"stage":          "compile",
				"reason":         conflict.Reason,
				"plugin":         conflict.PluginID,
				"existingPlugin": conflict.ExistingPlugin,
				"existingRoute":  conflict.ExistingRoute,
			},
		},
	}
}
