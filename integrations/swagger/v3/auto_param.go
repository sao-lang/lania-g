// auto_param.go 实现 swagger 集成对参数模型的自动推导逻辑。
package swagger

import (
	"reflect"
	"strings"

	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
	bindinghttp "github.com/sao-lang/lania-g/protocol/http/v3/binding"
)

func pathParameters(path string) []*Parameter {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	out := make([]*Parameter, 0)
	for _, part := range parts {
		if name, ok := strings.CutPrefix(part, ":"); ok && name != "" {
			out = append(out, &Parameter{Name: name, In: "path", Required: true, Schema: &Schema{Type: "string"}})
		}
	}
	return out
}

func parametersForRoute(def *httpadapter.RouteDefinition) []*Parameter {
	if def == nil || def.Controller == nil || def.MethodName == "" {
		if def == nil {
			return nil
		}
		return pathParameters(def.Path)
	}
	out := pathParameters(def.Path)
	rt := reflect.TypeOf(def.Controller)
	method, ok := rt.MethodByName(def.MethodName)
	if !ok {
		return out
	}
	pathNames := extractPathParamNames(def.Path)
	pathIndex := 0
	for i := 1; i < method.Type.NumIn(); i++ {
		out = append(out, parametersForType(method.Type.In(i), &pathIndex, pathNames)...)
	}
	return dedupeParameters(out)
}

func parametersForType(t reflect.Type, pathIndex *int, pathNames []string) []*Parameter {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() == reflect.Struct && t.PkgPath() == bindinghttp.PackagePath() {
		return parametersForHTTPWrapper(t, pathIndex, pathNames)
	}
	if t.Kind() == reflect.Struct && t.PkgPath() != bindinghttp.PackagePath() {
		return parametersForCompositeStruct(t)
	}
	return nil
}

func parametersForHTTPWrapper(t reflect.Type, pathIndex *int, pathNames []string) []*Parameter {
	name := trimGenericName(t.Name())
	switch name {
	case "Param":
		key := nextPathParam(pathIndex, pathNames)
		if key == "" {
			return nil
		}
		return []*Parameter{{Name: key, In: "path", Required: true, Schema: wrapperInnerSchema(t)}}
	case "Query":
		return []*Parameter{{In: "query", Required: false, Schema: wrapperInnerSchema(t)}}
	case "Header":
		return []*Parameter{{In: "header", Required: false, Schema: wrapperInnerSchema(t)}}
	case "Cookie":
		return []*Parameter{{In: "cookie", Required: false, Schema: wrapperInnerSchema(t)}}
	default:
		return nil
	}
}

func parametersForCompositeStruct(t reflect.Type) []*Parameter {
	out := make([]*Parameter, 0)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		ft := field.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() != reflect.Struct || ft.PkgPath() != bindinghttp.PackagePath() {
			continue
		}
		name := trimGenericName(ft.Name())
		switch name {
		case "Query":
			out = append(out, compositeParameter(field, bindinghttp.TagQuery, "query", ft, false))
		case "Param":
			out = append(out, compositeParameter(field, bindinghttp.TagParam, "path", ft, true))
		case "Header":
			out = append(out, compositeParameter(field, bindinghttp.TagHeader, "header", ft, false))
		case "Cookie":
			out = append(out, compositeParameter(field, bindinghttp.TagCookie, "cookie", ft, false))
		case "Form":
			out = append(out, compositeParameter(field, bindinghttp.TagForm, "query", ft, false))
		}
	}
	filtered := make([]*Parameter, 0, len(out))
	for _, item := range out {
		if item != nil && item.Name != "" {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func compositeParameter(field reflect.StructField, tagKey, in string, wrapperType reflect.Type, defaultRequired bool) *Parameter {
	key := field.Tag.Get(tagKey)
	if key == "" {
		key = strings.ToLower(field.Name)
	}
	required := defaultRequired
	if field.Tag.Get(bindinghttp.TagRequired) == "true" || field.Tag.Get(bindinghttp.TagRequired) == "1" {
		required = true
	}
	return &Parameter{
		Name:        key,
		In:          in,
		Required:    required,
		Description: field.Tag.Get("description"),
		Schema:      wrapperInnerSchema(wrapperType),
	}
}

func wrapperInnerSchema(t reflect.Type) *Schema {
	if t.NumField() == 0 {
		return &Schema{Type: "string"}
	}
	return inferSchema(t.Field(0).Type)
}

func extractPathParamNames(path string) []string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if name, ok := strings.CutPrefix(part, ":"); ok && name != "" {
			out = append(out, name)
		}
	}
	return out
}

func nextPathParam(index *int, names []string) string {
	if index == nil || *index >= len(names) {
		return ""
	}
	name := names[*index]
	*index++
	return name
}

func dedupeParameters(items []*Parameter) []*Parameter {
	seen := make(map[string]bool, len(items))
	out := make([]*Parameter, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		key := item.In + ":" + item.Name
		if key == ":" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, item)
	}
	return out
}
