package di

import "testing"

type typedContainerValue struct {
	Name string
}

type typedContainerContract interface {
	Value() string
}

type typedContainerImpl struct{}

func (typedContainerImpl) Value() string { return "ok" }

func TestMustGetByType(t *testing.T) {
	container := NewContainer()
	value := &typedContainerValue{Name: "demo"}
	contract := typedContainerContract(typedContainerImpl{})

	container.RegisterValue(typeToken[*typedContainerValue](), value)
	container.RegisterValue(typeToken[typedContainerContract](), contract)

	gotValue := MustGetByType[*typedContainerValue](container)
	if gotValue != value {
		t.Fatalf("value mismatch: %#v", gotValue)
	}

	gotContract := MustGetByType[typedContainerContract](container)
	if gotContract.Value() != "ok" {
		t.Fatalf("contract mismatch: %s", gotContract.Value())
	}
}
