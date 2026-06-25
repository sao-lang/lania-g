// schema_build.go 负责从声明与运行时信息构建 GraphQL schema。
package graphql

import (
	"fmt"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

func buildCompiledSchemaFromRegistry(reg *registry.Registry, local *Schema) (*compiledSchema, error) {
	if reg == nil {
		return nil, fmt.Errorf("graphql schema build requires registry")
	}
	cfg := collectConfigDecl(reg)
	if local != nil {
		cfg.Schema = local
	}

	schema := newCompiledSchema(cfg)
	ensureBuiltInObjects(schema)
	mergeDeclaredSchemaObjects(schema, cfg.Schema)

	for _, item := range reg.ListDecl(AdapterID, "resolvers") {
		resolverDef, ok := item.(*ResolverDefinition)
		if !ok || resolverDef == nil {
			continue
		}
		if resolverDef.Name != "" {
			ensureObject(schema, resolverDef.Name)
		}
		for _, field := range resolverDef.Fields {
			if field == nil {
				continue
			}
			parentName, err := resolveParentTypeName(resolverDef, field)
			if err != nil {
				return nil, err
			}
			parent := ensureObject(schema, parentName)
			if parent == nil {
				return nil, fmt.Errorf("graphql parent type %s not found", parentName)
			}
			compiled := buildCompiledField(schema, resolverDef, field, lookupFieldSchema(cfg.Schema, parentName, field.FieldName))
			compiled.ParentType = parentName
			bindReturnModelType(schema, compiled, field)
			parent.Fields[field.FieldName] = compiled
		}
	}

	return schema, nil
}

func buildCompiledSchemaCompat(local *Schema) (*compiledSchema, error) {
	return buildCompiledSchemaFromRegistry(registry.Global(), local)
}

func collectConfigDecl(reg *registry.Registry) *ConfigDecl {
	cfg := &ConfigDecl{}
	for _, item := range reg.ListDecl(AdapterID, "config") {
		decl, ok := item.(*ConfigDecl)
		if !ok || decl == nil {
			continue
		}
		if decl.Schema != nil {
			cfg.Schema = decl.Schema
		}
		if decl.DisableIntrospection {
			cfg.DisableIntrospection = true
		}
		if decl.ComplexityLimit > 0 {
			cfg.ComplexityLimit = decl.ComplexityLimit
		}
		if len(decl.Extensions) > 0 {
			cfg.Extensions = append(cfg.Extensions, decl.Extensions...)
		}
	}
	return cfg
}

func newCompiledSchema(cfg *ConfigDecl) *compiledSchema {
	schema := &compiledSchema{
		Objects:              make(map[string]*compiledObject),
		ScalarNames:          defaultScalarNames(),
		DisableIntrospection: cfg.DisableIntrospection,
		ComplexityLimit:      cfg.ComplexityLimit,
		Extensions:           append([]Extension{}, cfg.Extensions...),
	}
	if cfg.Schema != nil {
		for name := range cfg.Schema.ScalarNames {
			schema.ScalarNames[name] = true
		}
	}
	return schema
}

func ensureBuiltInObjects(schema *compiledSchema) {
	ensureObject(schema, string(FieldTypeQuery))
	ensureObject(schema, string(FieldTypeMutation))
}

func ensureObject(schema *compiledSchema, name string) *compiledObject {
	if schema == nil || name == "" {
		return nil
	}
	if existing, ok := schema.Objects[name]; ok {
		return existing
	}
	obj := &compiledObject{Name: name, Fields: make(map[string]*compiledField)}
	schema.Objects[name] = obj
	switch name {
	case string(FieldTypeQuery):
		schema.Query = obj
	case string(FieldTypeMutation):
		schema.Mutation = obj
	}
	return obj
}

func mergeDeclaredSchemaObjects(schema *compiledSchema, source *Schema) {
	if schema == nil || source == nil {
		return
	}
	for name, obj := range source.Objects {
		target := ensureObject(schema, name)
		mergeObjectSchema(target, obj)
	}
	if source.Query != nil {
		mergeObjectSchema(schema.Query, source.Query)
	}
	if source.Mutation != nil {
		mergeObjectSchema(schema.Mutation, source.Mutation)
	}
}

func resolveParentTypeName(resolverDef *ResolverDefinition, field *FieldDefinition) (string, error) {
	parentName := resolverDef.Name
	switch field.FieldType {
	case FieldTypeQuery:
		parentName = string(FieldTypeQuery)
	case FieldTypeMutation:
		parentName = string(FieldTypeMutation)
	case FieldTypeSubscription:
		return "", fmt.Errorf("graphql subscription %s.%s is not supported yet", resolverDef.Name, field.FieldName)
	}
	return parentName, nil
}

func bindReturnModelType(schema *compiledSchema, compiled *compiledField, field *FieldDefinition) {
	if compiled == nil || compiled.ReturnType == nil || compiled.ReturnType.Scalar {
		return
	}
	obj := ensureObject(schema, compiled.ReturnType.Name)
	if obj == nil || obj.ModelType != nil {
		return
	}
	obj.ModelType = inferResultGoType(field.Handler)
}

func buildCompiledField(schema *compiledSchema, resolverDef *ResolverDefinition, field *FieldDefinition, override *FieldSchema) *compiledField {
	out := &compiledField{
		Name:         field.FieldName,
		ResolverName: resolverDef.Name,
		Definition:   field,
		Args:         make(map[string]*compiledArg),
		Complexity:   maxInt(1, field.Complexity),
	}
	if override != nil && override.Complexity > 0 {
		out.Complexity = override.Complexity
	}
	mergeFieldArguments(out, field, override)
	if applyOverrideReturnType(schema, out, override) {
		return out
	}
	if applyDeclaredReturnType(schema, out, field) {
		return out
	}
	out.ReturnType = inferReturnType(field.Handler)
	if out.ReturnType != nil && out.ReturnType.Name == "JSON" && out.ReturnType.Scalar && resolverHasObjectFields(resolverDef) {
		out.ReturnType = &compiledTypeRef{Name: resolverDef.Name, Scalar: false}
	}
	if out.ReturnType != nil && !out.ReturnType.Scalar {
		if obj := schema.object(out.ReturnType.Name); obj != nil && obj.ModelType == nil {
			obj.ModelType = inferResultGoType(field.Handler)
		}
	}
	return out
}

func mergeFieldArguments(out *compiledField, field *FieldDefinition, override *FieldSchema) {
	for _, arg := range field.Args {
		if arg == nil {
			continue
		}
		out.Args[arg.Name] = &compiledArg{
			Name:         arg.Name,
			TypeName:     "JSON",
			NonNull:      arg.Required,
			Description:  arg.Description,
			DefaultValue: arg.DefaultValue,
		}
	}
	if override == nil {
		return
	}
	for name, arg := range override.Args {
		if arg == nil {
			continue
		}
		out.Args[name] = &compiledArg{
			Name:         name,
			TypeName:     firstNonEmpty(arg.TypeName, "JSON"),
			List:         arg.List,
			NonNull:      arg.NonNull,
			Description:  arg.Description,
			DefaultValue: arg.DefaultValue,
		}
	}
}

func applyOverrideReturnType(schema *compiledSchema, out *compiledField, override *FieldSchema) bool {
	if override == nil || override.TypeName == "" {
		return false
	}
	out.ReturnType = &compiledTypeRef{
		Name:    override.TypeName,
		List:    override.List,
		NonNull: override.NonNull,
		Scalar:  schema.ScalarNames[override.TypeName],
	}
	return true
}

func applyDeclaredReturnType(schema *compiledSchema, out *compiledField, field *FieldDefinition) bool {
	if field.Returns == "" {
		return false
	}
	inferredModel := inferResultGoType(field.Handler)
	out.ReturnType = &compiledTypeRef{Name: field.Returns, Scalar: schema.ScalarNames[field.Returns]}
	if !out.ReturnType.Scalar && inferredModel != nil {
		if obj := schema.object(out.ReturnType.Name); obj != nil && obj.ModelType == nil {
			obj.ModelType = inferredModel
		}
	}
	return true
}

func lookupFieldSchema(schema *Schema, parentName, fieldName string) *FieldSchema {
	if schema == nil {
		return nil
	}
	switch parentName {
	case string(FieldTypeQuery):
		if schema.Query != nil {
			return schema.Query.Fields[fieldName]
		}
	case string(FieldTypeMutation):
		if schema.Mutation != nil {
			return schema.Mutation.Fields[fieldName]
		}
	default:
		if obj := schema.Objects[parentName]; obj != nil {
			return obj.Fields[fieldName]
		}
	}
	return nil
}

func mergeObjectSchema(dst *compiledObject, src *ObjectSchema) {
	if dst == nil || src == nil {
		return
	}
	if dst.Fields == nil {
		dst.Fields = make(map[string]*compiledField)
	}
	for name, field := range src.Fields {
		if field == nil {
			continue
		}
		if existing, ok := dst.Fields[name]; ok && existing != nil {
			if field.TypeName != "" {
				existing.ReturnType = &compiledTypeRef{Name: field.TypeName, List: field.List, NonNull: field.NonNull}
			}
			continue
		}
		dst.Fields[name] = &compiledField{
			Name:       name,
			Definition: &FieldDefinition{FieldName: name},
			ReturnType: &compiledTypeRef{Name: firstNonEmpty(field.TypeName, "JSON")},
			Args:       make(map[string]*compiledArg),
			Complexity: maxInt(1, field.Complexity),
		}
	}
}
