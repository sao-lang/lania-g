// schema_infer.go 实现 GraphQL schema 类型推断辅助。
package graphql

import (
	"reflect"
	"strings"
	"time"
)

func inferReturnType(handler any) *compiledTypeRef {
	if handler == nil {
		return &compiledTypeRef{Name: "JSON", Scalar: true}
	}
	t := reflect.TypeOf(handler)
	if t.Kind() != reflect.Func || t.NumOut() == 0 {
		return &compiledTypeRef{Name: "JSON", Scalar: true}
	}
	out := unwrapResultType(t.Out(0))
	list := isListType(t.Out(0))
	if isScalarGoType(out) {
		return &compiledTypeRef{Name: scalarNameForType(out), Scalar: true, List: list}
	}
	if out.Name() != "" {
		return &compiledTypeRef{Name: out.Name(), Scalar: false, List: list}
	}
	return &compiledTypeRef{Name: "JSON", Scalar: true, List: list}
}

func inferResultGoType(handler any) reflect.Type {
	if handler == nil {
		return nil
	}
	t := reflect.TypeOf(handler)
	if t.Kind() != reflect.Func || t.NumOut() == 0 {
		return nil
	}
	out := unwrapResultType(t.Out(0))
	if out.Kind() == reflect.Struct {
		return out
	}
	return nil
}

// modelField 是模型 struct 展开后的一个字段（含嵌入字段摊平结果）。
type modelField struct {
	name string
	typ  reflect.Type
}

// collectModelFields 收集 struct 的可暴露字段。
//
// 规则与 encoding/json 的常见语义保持一致：
// - 跳过未导出字段
// - `json:"-"` 直接忽略
// - 匿名嵌入 struct（且未显式指定 json 名称）会被摊平到父类型，避免 GraphQL 里出现 baseResponse 这类包装层
// - 显式 json 名称优先于字段名
//
// visited 用于阻断自引用类型导致的无限递归。
func collectModelFields(t reflect.Type, visited map[reflect.Type]bool) []modelField {
	t = unwrapPointers(t)
	if t.Kind() != reflect.Struct || visited[t] {
		return nil
	}
	visited[t] = true

	out := make([]modelField, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}

		tag := sf.Tag.Get("json")
		if tag == "-" {
			continue
		}

		base := unwrapPointers(sf.Type)
		if sf.Anonymous && jsonName(tag) == "" && base.Kind() == reflect.Struct {
			out = append(out, collectModelFields(base, visited)...)
			continue
		}

		out = append(out, modelField{name: fieldCandidateName(sf), typ: sf.Type})
	}
	return out
}

// jsonName 提取 json tag 里的字段名部分。
func jsonName(tag string) string {
	if tag == "" {
		return ""
	}
	if idx := strings.IndexByte(tag, ','); idx >= 0 {
		tag = tag[:idx]
	}
	if tag == "-" {
		return ""
	}
	return tag
}

// ensureModelObject 为 struct 类型注册（或补齐）一个 GraphQL object，并递归注册其嵌套类型。
//
// 这样 "RegisterResult -> accountView -> ..." 这类嵌套选择集才可被解析。
// 自引用通过 "先写 ModelType 再递归" 的方式自然终止。
func ensureModelObject(schema *compiledSchema, t reflect.Type) *compiledObject {
	if schema == nil || t == nil {
		return nil
	}

	t = unwrapPointers(t)
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		return ensureModelObject(schema, t.Elem())
	case reflect.Map:
		// map[string]any 之类的动态结构按 JSON 标量处理，不做对象展开。
		return nil
	case reflect.Struct:
	default:
		return nil
	}

	if isScalarGoType(t) || t.Name() == "" {
		return nil
	}

	obj := ensureObject(schema, t.Name())
	if obj == nil {
		return nil
	}
	if obj.ModelType != nil {
		return obj
	}
	obj.ModelType = t

	for _, mf := range collectModelFields(t, make(map[reflect.Type]bool)) {
		ensureModelObject(schema, mf.typ)
	}
	return obj
}

// registerNestedModelTypes 为已绑定 ModelType 的 object 继续注册其嵌套 struct 类型。
//
// 用于 schema 构建阶段：确保校验/内省在遇到嵌套选择集时能找到对应 object。
func registerNestedModelTypes(schema *compiledSchema, t reflect.Type) {
	if schema == nil || t == nil {
		return
	}
	t = unwrapPointers(t)
	if t.Kind() != reflect.Struct || isScalarGoType(t) || t.Name() == "" {
		return
	}
	if obj := ensureObject(schema, t.Name()); obj != nil && obj.ModelType == nil {
		obj.ModelType = t
	}
	for _, mf := range collectModelFields(t, make(map[reflect.Type]bool)) {
		ensureModelObject(schema, mf.typ)
	}
}

func implicitFieldForModel(schema *compiledSchema, obj *compiledObject, fieldName string) *compiledField {
	if schema == nil || obj == nil || obj.ModelType == nil {
		return nil
	}
	t := unwrapPointers(obj.ModelType)
	if t.Kind() != reflect.Struct {
		return nil
	}
	for _, mf := range collectModelFields(t, make(map[reflect.Type]bool)) {
		if mf.name != fieldName {
			continue
		}
		ref := inferTypeRefFromStructField(mf.typ)
		if !ref.Scalar {
			// 把嵌套 struct（或 struct 列表）递归注册成 object，供后续选择集解析。
			ensureModelObject(schema, mf.typ)
		}
		return &compiledField{
			ParentType: obj.Name,
			Name:       fieldName,
			Definition: &FieldDefinition{FieldName: fieldName},
			ReturnType: ref,
			Args:       map[string]*compiledArg{},
			Complexity: 1,
		}
	}
	return nil
}

func inferTypeRefFromStructField(t reflect.Type) *compiledTypeRef {
	list := false
	t = unwrapPointers(t)
	if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		list = true
		t = unwrapPointers(t.Elem())
	}
	if isScalarGoType(t) {
		return &compiledTypeRef{Name: scalarNameForType(t), Scalar: true, List: list}
	}
	name := t.Name()
	if name == "" {
		name = "JSON"
	}
	return &compiledTypeRef{Name: name, Scalar: name == "JSON", List: list}
}

func unwrapResultType(t reflect.Type) reflect.Type {
	t = unwrapPointers(t)
	if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		return unwrapPointers(t.Elem())
	}
	return t
}

func isListType(t reflect.Type) bool {
	t = unwrapPointers(t)
	return t.Kind() == reflect.Slice || t.Kind() == reflect.Array
}

func unwrapPointers(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}

func fieldCandidateName(sf reflect.StructField) string {
	candidate := sf.Name
	if tag := jsonName(sf.Tag.Get("json")); tag != "" {
		candidate = tag
	} else {
		candidate = toLowerCamel(sf.Name)
	}
	return candidate
}

func defaultScalarNames() map[string]bool {
	return map[string]bool{
		"String":  true,
		"Int":     true,
		"Float":   true,
		"Boolean": true,
		"ID":      true,
		"JSON":    true,
	}
}

func isScalarGoType(t reflect.Type) bool {
	if t == nil {
		return true
	}
	if t == reflect.TypeOf(time.Time{}) {
		return true
	}
	switch t.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64, reflect.String:
		return true
	}
	return false
}

func scalarNameForType(t reflect.Type) string {
	if t == nil {
		return "JSON"
	}
	if t == reflect.TypeOf(time.Time{}) {
		return "String"
	}
	switch t.Kind() {
	case reflect.Bool:
		return "Boolean"
	case reflect.Float32, reflect.Float64:
		return "Float"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "Int"
	case reflect.String:
		if strings.EqualFold(t.Name(), "ID") {
			return "ID"
		}
		return "String"
	default:
		return "JSON"
	}
}

func toLowerCamel(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func resolverHasObjectFields(def *ResolverDefinition) bool {
	if def == nil {
		return false
	}
	for _, field := range def.Fields {
		if field != nil && field.FieldType == FieldTypeObject {
			return true
		}
	}
	return false
}
