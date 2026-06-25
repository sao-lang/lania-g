// schema.go 定义 GraphQL adapter 使用的 schema 模型结构。
package graphql

import "reflect"

type compiledSchema struct {
	Query                *compiledObject
	Mutation             *compiledObject
	Objects              map[string]*compiledObject
	ScalarNames          map[string]bool
	DisableIntrospection bool
	ComplexityLimit      int
	Extensions           []Extension
}

type compiledObject struct {
	Name      string
	Fields    map[string]*compiledField
	ModelType reflect.Type
}

type compiledField struct {
	ParentType   string
	Name         string
	ResolverName string
	Definition   *FieldDefinition
	ReturnType   *compiledTypeRef
	Args         map[string]*compiledArg
	Complexity   int
}

type compiledArg struct {
	Name         string
	TypeName     string
	List         bool
	NonNull      bool
	Description  string
	DefaultValue any
}

type compiledTypeRef struct {
	Name    string
	List    bool
	NonNull bool
	Scalar  bool
}

func (s *compiledSchema) object(name string) *compiledObject {
	if s == nil {
		return nil
	}
	return s.Objects[name]
}

func (s *compiledSchema) field(parentType, fieldName string) *compiledField {
	parent := s.object(parentType)
	if parent == nil {
		return nil
	}
	if field := parent.Fields[fieldName]; field != nil {
		return field
	}
	return implicitFieldForModel(parent, fieldName)
}
