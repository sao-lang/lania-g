// schema_describe.go 负责把 GraphQL schema 结构转换为可读描述信息。
package graphql

import "sort"

func (s *compiledSchema) describeSchema() map[string]any {
	types := make([]map[string]any, 0, len(s.Objects)+len(s.ScalarNames))
	for name := range s.ScalarNames {
		types = append(types, map[string]any{"kind": "SCALAR", "name": name})
	}
	for _, obj := range s.sortedObjects() {
		fields := make([]map[string]any, 0, len(obj.Fields))
		for _, field := range sortedFields(obj.Fields) {
			args := make([]map[string]any, 0, len(field.Args))
			for _, arg := range sortedArgs(field.Args) {
				args = append(args, map[string]any{
					"name":         arg.Name,
					"description":  arg.Description,
					"defaultValue": arg.DefaultValue,
					"type":         describeType(fieldSchemaTypeName(arg.TypeName), arg.List, arg.NonNull),
				})
			}
			fields = append(fields, map[string]any{
				"name":              field.Name,
				"description":       field.Definition.Description,
				"deprecationReason": field.Definition.Deprecation,
				"args":              args,
				"type":              describeType(field.ReturnType.Name, field.ReturnType.List, field.ReturnType.NonNull),
			})
		}
		types = append(types, map[string]any{"kind": "OBJECT", "name": obj.Name, "fields": fields})
	}
	out := map[string]any{
		"queryType": map[string]any{"name": string(FieldTypeQuery)},
		"types":     types,
	}
	if s.Mutation != nil && len(s.Mutation.Fields) > 0 {
		out["mutationType"] = map[string]any{"name": string(FieldTypeMutation)}
	}
	return out
}

func (s *compiledSchema) describeTypeByName(name string) map[string]any {
	if s.ScalarNames[name] {
		return map[string]any{"kind": "SCALAR", "name": name}
	}
	obj := s.object(name)
	if obj == nil {
		return nil
	}
	fields := make([]map[string]any, 0, len(obj.Fields))
	for _, field := range sortedFields(obj.Fields) {
		fields = append(fields, map[string]any{
			"name": field.Name,
			"type": describeType(field.ReturnType.Name, field.ReturnType.List, field.ReturnType.NonNull),
		})
	}
	return map[string]any{"kind": "OBJECT", "name": name, "fields": fields}
}

func (s *compiledSchema) sortedObjects() []*compiledObject {
	items := make([]*compiledObject, 0, len(s.Objects))
	for _, obj := range s.Objects {
		if obj != nil {
			items = append(items, obj)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func sortedFields(m map[string]*compiledField) []*compiledField {
	items := make([]*compiledField, 0, len(m))
	for _, field := range m {
		if field != nil {
			items = append(items, field)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func sortedArgs(m map[string]*compiledArg) []*compiledArg {
	items := make([]*compiledArg, 0, len(m))
	for _, arg := range m {
		if arg != nil {
			items = append(items, arg)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func describeType(name string, list, nonNull bool) map[string]any {
	kind := "OBJECT"
	if defaultScalarNames()[name] {
		kind = "SCALAR"
	}
	base := map[string]any{"kind": kind, "name": name}
	if list {
		base = map[string]any{"kind": "LIST", "ofType": base}
	}
	if nonNull {
		base = map[string]any{"kind": "NON_NULL", "ofType": base}
	}
	return base
}

func fieldSchemaTypeName(name string) string {
	if name == "" {
		return "JSON"
	}
	return name
}
