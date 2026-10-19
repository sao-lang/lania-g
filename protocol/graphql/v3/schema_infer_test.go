package graphql

import (
	"reflect"
	"testing"
)

type InferBase struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Success bool   `json:"success"`
}

type InferAccount struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

type InferResult struct {
	InferBase
	Account InferAccount `json:"account"`
}

type InferSelfRef struct {
	Name    string        `json:"name"`
	Manager *InferSelfRef `json:"manager"`
}

func TestCollectModelFields_FlattensEmbeddedStruct(t *testing.T) {
	fields := collectModelFields(reflect.TypeFor[InferResult](), make(map[reflect.Type]bool))
	names := make(map[string]reflect.Type, len(fields))
	for _, f := range fields {
		names[f.name] = f.typ
	}

	for _, want := range []string{"code", "msg", "success", "account"} {
		if _, ok := names[want]; !ok {
			t.Fatalf("missing field %q, got %v", want, names)
		}
	}
	if names["account"] != reflect.TypeFor[InferAccount]() {
		t.Fatalf("account type = %v", names["account"])
	}
}

func TestCollectModelFields_SkipsJSONDash(t *testing.T) {
	type withSkip struct {
		Visible string `json:"visible"`
		Hidden  string `json:"-"`
	}
	fields := collectModelFields(reflect.TypeFor[withSkip](), make(map[reflect.Type]bool))
	if len(fields) != 1 || fields[0].name != "visible" {
		t.Fatalf("fields = %#v", fields)
	}
}

func TestEnsureModelObject_RegistersNestedTypes(t *testing.T) {
	schema := newCompiledSchema(&ConfigDecl{})

	obj := ensureModelObject(schema, reflect.TypeFor[*InferResult]())
	if obj == nil || obj.Name != "InferResult" {
		t.Fatalf("obj = %#v", obj)
	}
	if obj.ModelType != reflect.TypeFor[InferResult]() {
		t.Fatalf("model type = %v", obj.ModelType)
	}

	account := schema.object("InferAccount")
	if account == nil {
		t.Fatalf("nested object InferAccount not registered")
	}
	if account.ModelType != reflect.TypeFor[InferAccount]() {
		t.Fatalf("nested model type = %v", account.ModelType)
	}
}

func TestEnsureModelObject_HandlesSelfReference(t *testing.T) {
	schema := newCompiledSchema(&ConfigDecl{})

	obj := ensureModelObject(schema, reflect.TypeFor[InferSelfRef]())
	if obj == nil {
		t.Fatalf("obj is nil")
	}

	field := implicitFieldForModel(schema, obj, "manager")
	if field == nil {
		t.Fatalf("manager field not inferred")
	}
	if field.ReturnType == nil || field.ReturnType.Scalar {
		t.Fatalf("manager return type = %#v", field.ReturnType)
	}
	if field.ReturnType.Name != "InferSelfRef" {
		t.Fatalf("manager type name = %s", field.ReturnType.Name)
	}
}

func TestImplicitFieldForModel_ResolvesFlattenedField(t *testing.T) {
	schema := newCompiledSchema(&ConfigDecl{})
	obj := ensureModelObject(schema, reflect.TypeFor[InferResult]())

	for _, name := range []string{"code", "msg", "success"} {
		field := implicitFieldForModel(schema, obj, name)
		if field == nil {
			t.Fatalf("field %q not inferred", name)
		}
		if field.ReturnType == nil || !field.ReturnType.Scalar {
			t.Fatalf("field %q should be scalar, got %#v", name, field.ReturnType)
		}
	}

	account := implicitFieldForModel(schema, obj, "account")
	if account == nil || account.ReturnType == nil || account.ReturnType.Scalar {
		t.Fatalf("account field = %#v", account)
	}
	if schema.object("InferAccount") == nil {
		t.Fatalf("nested object not registered on demand")
	}
}
