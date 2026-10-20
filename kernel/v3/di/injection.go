// injection.go 实现字段注入的审计与严格模式。
//
// 字段注入是"静默"的：容器遍历 struct 的导出零值字段，找到类型就填，
// 找不到就跳过。用户最终只看到"某个字段是 nil"，栈里没有任何线索。
//
// 这里记录每一次尝试，并提供 StrictInjection 让缺失依赖在启动期就报错。
package di

import (
	"fmt"
	"reflect"
	"strings"
)

// InjectionStatus 表示一次字段注入尝试的结果。
type InjectionStatus string

const (
	// InjectionInjected 表示成功注入。
	InjectionInjected InjectionStatus = "injected"
	// InjectionMissingProvider 表示容器（含 parent 链）里没有该类型的 provider。
	InjectionMissingProvider InjectionStatus = "missing-provider"
	// InjectionUnexported 表示字段未导出，无法注入。
	InjectionUnexported InjectionStatus = "unexported"
	// InjectionAlreadySet 表示字段已有非零值，按"不覆盖"规则跳过。
	InjectionAlreadySet InjectionStatus = "already-set"
	// InjectionNotSettable 表示字段不可写（典型场景：值类型的不可寻址副本）。
	InjectionNotSettable InjectionStatus = "not-settable"
	// InjectionTypeMismatch 表示解析出的实例类型与字段类型不匹配。
	InjectionTypeMismatch InjectionStatus = "type-mismatch"
	// InjectionOptional 表示字段用 `di:"optional"` 标记，缺失属预期。
	InjectionOptional InjectionStatus = "optional-missing"
	// InjectionSkipped 表示字段用 `di:"-"` 显式排除。
	InjectionSkipped InjectionStatus = "skipped"
)

// InjectionRecord 描述一次字段注入尝试。
type InjectionRecord struct {
	// Token 是字段类型（也就是期望的依赖 token）。
	Token reflect.Type
	// TargetType 是宿主 struct 类型。
	TargetType reflect.Type
	// Field 是字段名。
	Field string
	// Status 是本次尝试的结果。
	Status InjectionStatus
	// Reason 是人类可读的补充说明。
	Reason string
	// Candidates 在 MissingProvider 时给出容器内相近的 token，帮助定位"类型写错"。
	Candidates []reflect.Type
}

// String 以单行文本表示一次注入记录。
func (r InjectionRecord) String() string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%s.%s (%s): %s", targetName(r.TargetType), r.Field, typeName(r.Token), r.Status)
	if r.Reason != "" {
		fmt.Fprintf(&builder, " — %s", r.Reason)
	}
	if len(r.Candidates) > 0 {
		names := make([]string, 0, len(r.Candidates))
		for _, candidate := range r.Candidates {
			names = append(names, typeName(candidate))
		}
		fmt.Fprintf(&builder, " (nearest: %s)", strings.Join(names, ", "))
	}
	return builder.String()
}

// SetInjectionAudit 开启/关闭注入审计。关闭后可省掉少量内存分配。
func (c *Container) SetInjectionAudit(enabled bool) {
	if c == nil {
		return
	}
	c.auditOn.Store(enabled)
	if !enabled {
		c.auditMu.Lock()
		c.audit = nil
		c.auditMu.Unlock()
	}
}

// injectionAuditEnabled 返回审计开关（供 NewChild 继承）。
func (c *Container) injectionAuditEnabled() bool {
	if c == nil {
		return true
	}
	return c.auditOn.Load()
}

// SetStrictInjection 开启严格注入模式。
//
// 开启后，导出、可写、零值的字段如果没有对应 provider 会直接报错；
// 用 `di:"optional"` 标记的字段不受影响。
// 建议在测试与示例项目中开启，生产环境按需开启。
func (c *Container) SetStrictInjection(enabled bool) {
	if c == nil {
		return
	}
	c.strict.Store(enabled)
}

// StrictInjection 返回当前是否开启严格模式。
func (c *Container) StrictInjection() bool {
	if c == nil {
		return false
	}
	return c.strict.Load()
}

// InjectionReport 返回本容器（含 parent 链）的注入审计快照。
//
// 每个容器最多保留 maxInjectionRecords 条（超出部分丢弃），避免 request-scoped 依赖
// 在高频请求下让审计缓冲区无界增长。
func (c *Container) InjectionReport() []InjectionRecord {
	if c == nil {
		return nil
	}
	var out []InjectionRecord
	for cursor := c; cursor != nil; cursor = cursor.parent {
		cursor.auditMu.Lock()
		out = append(out, cursor.audit...)
		cursor.auditMu.Unlock()
	}
	return out
}

// InjectionProblems 返回需要处理的注入问题（缺失 / 类型不匹配）。
func (c *Container) InjectionProblems() []InjectionRecord {
	records := c.InjectionReport()
	out := make([]InjectionRecord, 0, len(records))
	for _, record := range records {
		if record.Status == InjectionMissingProvider || record.Status == InjectionTypeMismatch {
			out = append(out, record)
		}
	}
	return out
}

// maxInjectionRecords 限制单个容器的审计记录条数。
//
// 审计是诊断信息，不能无界增长：request-scoped / transient provider 每个请求都会产生记录，
// 若不设上限，长跑进程的内存会持续上涨。超过上限后只保留最初的记录（启动期那批最有诊断价值）。
const maxInjectionRecords = 512

func (c *Container) recordInjection(record InjectionRecord) {
	if c == nil {
		return
	}
	// 热路径快路径：审计关闭时不碰锁。
	if !c.auditOn.Load() {
		return
	}
	c.auditMu.Lock()
	defer c.auditMu.Unlock()
	if !c.auditOn.Load() || len(c.audit) >= maxInjectionRecords {
		return
	}
	c.audit = append(c.audit, record)
}

// injectionCandidates 在容器链里找出与目标类型"名字相近"的 token。
//
// 用途：区分"完全忘了注册"与"注册了但类型写错/没导出"。
func (c *Container) injectionCandidates(token reflect.Type) []reflect.Type {
	if token == nil {
		return nil
	}
	want := baseTypeName(token)
	if want == "" {
		return nil
	}

	seen := make(map[reflect.Type]bool)
	var out []reflect.Type
	for cursor := c; cursor != nil; cursor = cursor.parent {
		cursor.mu.RLock()
		for key := range cursor.providers {
			typ, ok := key.(reflect.Type)
			if !ok || seen[typ] {
				continue
			}
			seen[typ] = true
			if typ == token {
				continue
			}
			if strings.EqualFold(baseTypeName(typ), want) || strings.Contains(baseTypeName(typ), want) {
				out = append(out, typ)
			}
		}
		cursor.mu.RUnlock()
		if len(out) >= 5 {
			break
		}
	}
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

// fieldInjectionMode 解析 `di` tag。
//
//	di:"-"        跳过注入
//	di:"optional" 允许缺失
func fieldInjectionMode(tag string) (skip bool, optional bool) {
	for _, part := range strings.Split(tag, ",") {
		switch strings.TrimSpace(part) {
		case "-":
			skip = true
		case "optional":
			optional = true
		}
	}
	return skip, optional
}

func typeName(typ reflect.Type) string {
	if typ == nil {
		return "<nil>"
	}
	return typ.String()
}

func baseTypeName(typ reflect.Type) string {
	for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map {
		typ = typ.Elem()
	}
	return typ.Name()
}

func targetName(typ reflect.Type) string {
	if typ == nil {
		return "<unknown>"
	}
	if typ.Kind() == reflect.Ptr {
		typ = typ.Elem()
	}
	return typ.Name()
}
