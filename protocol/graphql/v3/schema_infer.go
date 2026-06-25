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

func implicitFieldForModel(obj *compiledObject, fieldName string) *compiledField {
	if obj == nil || obj.ModelType == nil {
		return nil
	}
	t := unwrapPointers(obj.ModelType)
	if t.Kind() != reflect.Struct {
		return nil
	}
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		candidate := fieldCandidateName(sf)
		if candidate != fieldName {
			continue
		}
		ref := inferTypeRefFromStructField(sf.Type)
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
	if tag := sf.Tag.Get("json"); tag != "" {
		if idx := strings.IndexByte(tag, ','); idx >= 0 {
			tag = tag[:idx]
		}
		if tag != "" {
			candidate = tag
		}
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
