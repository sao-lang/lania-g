// auto_body.go 实现 swagger 集成对请求体模型的自动推导逻辑。
package swagger

import (
	"reflect"
	"strings"

	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
	bindinghttp "github.com/sao-lang/lania-g/protocol/http/v3/binding"
)

func requestBodyForRoute(def *httpadapter.RouteDefinition) *RequestBody {
	if def == nil || def.Controller == nil || def.MethodName == "" {
		return nil
	}
	switch def.Method {
	case httpadapter.POST, httpadapter.PUT, httpadapter.PATCH:
	default:
		return nil
	}
	rt := reflect.TypeOf(def.Controller)
	method, ok := rt.MethodByName(def.MethodName)
	if !ok {
		return defaultJSONBody()
	}
	for i := 1; i < method.Type.NumIn(); i++ {
		param := method.Type.In(i)
		if schema := schemaForBodyParam(param); schema != nil {
			return &RequestBody{Required: true, Content: map[string]*MediaType{"application/json": {Schema: schema}}}
		}
	}
	return nil
}

func schemaForBodyParam(param reflect.Type) *Schema {
	t := param
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() == reflect.Struct && t.PkgPath() == bindinghttp.PackagePath() {
		name := trimGenericName(t.Name())
		if name == "Body" || name == "BodyAs" || name == "MustBodyAs" || name == "Bind" || name == "MustBind" {
			field := t.Field(0)
			return inferSchema(field.Type)
		}
		return nil
	}
	if t.Kind() == reflect.Struct && t.PkgPath() != bindinghttp.PackagePath() {
		if hasCompositeBindingFields(t) {
			return schemaForCompositeBody(t)
		}
		return inferSchema(param)
	}
	return nil
}

func hasCompositeBindingFields(t reflect.Type) bool {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		ft := field.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && ft.PkgPath() == bindinghttp.PackagePath() {
			return true
		}
	}
	return false
}

func schemaForCompositeBody(t reflect.Type) *Schema {
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
		if name != "Body" && name != "BodyAs" && name != "MustBodyAs" && name != "Bind" && name != "MustBind" {
			continue
		}
		if bodySchema := wrapperInnerSchema(ft); bodySchema != nil {
			return bodySchema
		}
	}
	return nil
}

func trimGenericName(name string) string {
	if idx := strings.Index(name, "["); idx >= 0 {
		return name[:idx]
	}
	return name
}
