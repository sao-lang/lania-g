// helpers.go 提供 GraphQL adapter 内部复用的杂项辅助函数。
package graphql

import (
	"net/http"
	"reflect"
	"strings"

	gqlast "github.com/graphql-go/graphql/language/ast"
)

func fieldKey(field *gqlast.Field) string {
	if field.Alias != nil && field.Alias.Value != "" {
		return field.Alias.Value
	}
	if field.Name != nil {
		return field.Name.Value
	}
	return ""
}

func selectionFieldNames(set *gqlast.SelectionSet) []string {
	if set == nil {
		return nil
	}
	out := make([]string, 0, len(set.Selections))
	for _, selection := range set.Selections {
		if field, ok := selection.(*gqlast.Field); ok && field.Name != nil {
			out = append(out, field.Name.Value)
		}
	}
	return out
}

func appendPath(path []string, item string) []string {
	out := make([]string, 0, len(path)+1)
	out = append(out, path...)
	out = append(out, item)
	return out
}

func (a *Adapter) addError(opCtx *OperationContext, err error, path []string) {
	if err == nil {
		return
	}
	if gqlErr, ok := err.(*GraphQLError); ok {
		if len(gqlErr.Path) == 0 {
			gqlErr.Path = append([]string{}, path...)
		}
		opCtx.Errors = append(opCtx.Errors, gqlErr)
		return
	}
	opCtx.Errors = append(opCtx.Errors, &GraphQLError{Message: err.Error(), Path: append([]string{}, path...)})
}

func (a *Adapter) formatError(err error) any {
	if err == nil {
		return nil
	}
	if a.errorFormatter != nil {
		return a.errorFormatter(err)
	}
	if gqlErr, ok := err.(*GraphQLError); ok {
		return gqlErr
	}
	return &GraphQLError{Message: err.Error()}
}

func projectSelection(value any, sel *gqlast.SelectionSet) any {
	if value == nil || sel == nil {
		return value
	}
	rv := reflect.ValueOf(value)
	for rv.IsValid() && rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		out := make([]any, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out = append(out, projectSelection(rv.Index(i).Interface(), sel))
		}
		return out
	case reflect.Map, reflect.Struct:
		out := map[string]any{}
		for _, selection := range sel.Selections {
			field, ok := selection.(*gqlast.Field)
			if !ok || field.Name == nil {
				continue
			}
			name := field.Name.Value
			key := fieldKey(field)
			raw := extractFieldValue(rv, name)
			if field.SelectionSet != nil && len(field.SelectionSet.Selections) > 0 {
				out[key] = projectSelection(raw, field.SelectionSet)
			} else {
				out[key] = raw
			}
		}
		return out
	default:
		return value
	}
}

func extractFieldValue(rv reflect.Value, name string) any {
	if !rv.IsValid() {
		return nil
	}
	for rv.IsValid() && rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Map:
		key := reflect.ValueOf(name)
		if val := rv.MapIndex(key); val.IsValid() {
			return val.Interface()
		}
		snake := toSnakeCase(name)
		if val := rv.MapIndex(reflect.ValueOf(snake)); val.IsValid() {
			return val.Interface()
		}
	case reflect.Struct:
		if field := rv.FieldByNameFunc(func(s string) bool { return strings.EqualFold(s, name) }); field.IsValid() {
			return field.Interface()
		}
	}
	return nil
}

func copyHeaderValues(dstSingle map[string]string, dstMulti map[string][]string, hdr http.Header) {
	for k, v := range hdr {
		if len(v) > 0 {
			dstSingle[k] = v[0]
			dstMulti[k] = append([]string{}, v...)
		}
	}
}

func headerSnapshot(h http.Header) map[string][]string {
	out := make(map[string][]string, len(h))
	for k, v := range h {
		out[k] = append([]string{}, v...)
	}
	return out
}

func copyStringAnyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func toSnakeCase(s string) string {
	if len(s) == 0 {
		return s
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && 'A' <= r && r <= 'Z' {
			b.WriteByte('_')
		}
		if 'A' <= r && r <= 'Z' {
			r += 32
		}
		b.WriteRune(r)
	}
	return b.String()
}
