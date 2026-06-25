// auto_route.go 实现 swagger 集成对路由声明的自动提取逻辑。
package swagger

import (
	"reflect"
	"strings"

	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
)

func ensurePath(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func statusKey(code int) string {
	if code <= 0 {
		return "200"
	}
	return strconvItoa(code)
}

func httpOperationID(def *httpadapter.RouteDefinition) string {
	if def == nil {
		return ""
	}
	method := strings.ToLower(string(def.Method))
	name := def.MethodName
	if name == "" {
		name = method
	}
	path := strings.Trim(def.Path, "/")
	path = strings.ReplaceAll(path, "/", "_")
	path = strings.ReplaceAll(path, ":", "")
	if path == "" {
		return method + "_" + name
	}
	return method + "_" + path + "_" + name
}

func responseSpecForRoute(def *httpadapter.RouteDefinition) map[string]*Response {
	response := &Response{Description: "OK"}
	if def == nil || def.Controller == nil || def.MethodName == "" {
		code := 200
		if def != nil {
			code = def.StatusCode
		}
		return map[string]*Response{statusKey(code): response}
	}
	code := statusKey(def.StatusCode)
	rt := reflect.TypeOf(def.Controller)
	method, ok := rt.MethodByName(def.MethodName)
	if !ok {
		return map[string]*Response{code: response}
	}
	for i := 0; i < method.Type.NumOut(); i++ {
		outType := method.Type.Out(i)
		if isErrorType(outType) {
			continue
		}
		schema := inferSchema(outType)
		if wrapped := wrapResponseSchema(def, schema); wrapped != nil {
			schema = wrapped
		}
		response.Content = map[string]*MediaType{
			"application/json": {Schema: schema},
		}
		break
	}
	responses := map[string]*Response{code: response}
	if bldr := currentBuilder(); bldr != nil {
		for _, status := range bldr.defaultErrorCodes {
			responses[statusKey(status)] = bldr.defaultErrorResponse()
		}
	}
	if def.Doc != nil {
		for status, description := range def.Doc.ErrorResponses {
			errResp := &Response{Description: description}
			if bldr := currentBuilder(); bldr != nil && bldr.defaultErrorSchema != nil {
				errResp.Content = map[string]*MediaType{
					"application/json": {Schema: bldr.defaultErrorSchema},
				}
			}
			responses[statusKey(status)] = errResp
		}
	}
	return responses
}

func assignOperation(path *Path, method httpadapter.Method, op *Operation) {
	switch method {
	case httpadapter.GET:
		path.Get = op
	case httpadapter.POST:
		path.Post = op
	case httpadapter.PUT:
		path.Put = op
	case httpadapter.DELETE:
		path.Delete = op
	case httpadapter.PATCH:
		path.Patch = op
	case httpadapter.HEAD:
		path.Head = op
	case httpadapter.OPTIONS:
		path.Options = op
	case httpadapter.ALL:
		path.Get = op
		path.Post = op
		path.Put = op
		path.Delete = op
		path.Patch = op
	}
}

func routeSummary(def *httpadapter.RouteDefinition) string {
	if def != nil && def.Doc != nil && def.Doc.Summary != "" {
		return def.Doc.Summary
	}
	if def == nil {
		return ""
	}
	return def.MethodName
}

func routeDescription(def *httpadapter.RouteDefinition) string {
	if def != nil && def.Doc != nil {
		return def.Doc.Description
	}
	return ""
}

func routeTags(def *httpadapter.RouteDefinition) []string {
	if def != nil && def.Doc != nil && len(def.Doc.Tags) > 0 {
		return append([]string{}, def.Doc.Tags...)
	}
	return nil
}

func routeSecurity(def *httpadapter.RouteDefinition) []map[string][]string {
	if def == nil || def.Doc == nil || len(def.Doc.Security) == 0 {
		return nil
	}
	out := make([]map[string][]string, 0, len(def.Doc.Security))
	for _, item := range def.Doc.Security {
		out = append(out, map[string][]string{item.Name: append([]string{}, item.Scopes...)})
	}
	return out
}

func wrapResponseSchema(def *httpadapter.RouteDefinition, payload *Schema) *Schema {
	if def == nil || def.Doc == nil || def.Doc.ResponseType == nil {
		return payload
	}
	envelope := inferSchema(reflect.TypeOf(def.Doc.ResponseType))
	if def.Doc.ResponseField == "" || envelope == nil || envelope.Properties == nil {
		return envelope
	}
	envelope.Properties[def.Doc.ResponseField] = payload
	return envelope
}

func defaultJSONBody() *RequestBody {
	return &RequestBody{
		Required: true,
		Content: map[string]*MediaType{
			"application/json": {Schema: &Schema{Type: "object"}},
		},
	}
}
