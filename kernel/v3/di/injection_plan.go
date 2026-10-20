// injection_plan.go 缓存「某个 struct 类型有哪些字段需要注入」的静态信息。
//
// 字段是否导出、`di` 标签怎么写，只与类型有关，与实例无关。
// 之前每次构造都要重新 `Type().Field(i)`（连 struct tag 一起解析）一次，
// 对 request-scoped / transient provider 来说就是每请求重复做同一件事。
package di

import (
	"reflect"
	"sync"
)

// injectionTarget 描述一个字段的注入计划（纯类型信息，可安全共享）。
type injectionTarget struct {
	// Index 是字段在 struct 中的下标。
	Index int
	// Name 是字段名（用于审计与错误信息）。
	Name string
	// Type 是字段类型，同时作为依赖 token。
	Type reflect.Type
	// Exported 表示字段可被注入（首字母大写）。
	Exported bool
	// Skip 对应 `di:"-"`。
	Skip bool
	// Optional 对应 `di:"optional"`。
	Optional bool
}

// injectionPlanCache 缓存 reflect.Type -> []injectionTarget。
//
// 类型一旦确定就不会再变，因此该缓存无需失效逻辑；用 sync.Map 避免读路径加锁。
var injectionPlanCache sync.Map

// injectionPlanFor 返回某个 struct 类型的注入计划（首次调用时计算并缓存）。
func injectionPlanFor(typ reflect.Type) []injectionTarget {
	if cached, ok := injectionPlanCache.Load(typ); ok {
		return cached.([]injectionTarget)
	}

	plan := make([]injectionTarget, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		skip, optional := fieldInjectionMode(field.Tag.Get("di"))
		plan = append(plan, injectionTarget{
			Index:    i,
			Name:     field.Name,
			Type:     field.Type,
			Exported: field.IsExported(),
			Skip:     skip,
			Optional: optional,
		})
	}

	actual, _ := injectionPlanCache.LoadOrStore(typ, plan)
	return actual.([]injectionTarget)
}
