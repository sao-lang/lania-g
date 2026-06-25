package module

import (
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/di"
)

type typedValue struct {
	Name string
}

type typedContract interface {
	Value() string
}

type typedImpl struct{}

func (typedImpl) Value() string { return "ok" }

// 这个用例验证：泛型版 MustGetByType 能按类型 token 正确从 ModuleRef 中解析实例。
func TestMustGetByType(t *testing.T) {
	value := &typedValue{Name: "demo"}
	contract := typedContract(typedImpl{})

	p1, err := di.ProviderFromInstanceWithToken(typeToken[*typedValue](), value, di.Singleton)
	if err != nil {
		t.Fatalf("provider typed value: %v", err)
	}
	p2, err := di.ProviderFromInstanceWithToken(typeToken[typedContract](), contract, di.Singleton)
	if err != nil {
		t.Fatalf("provider typed contract: %v", err)
	}
	root := CreateModule(nil, []*di.Provider{p1, p2}, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init root: %v", err)
	}
	ref := NewModuleRef(root)

	gotValue := MustGetByType[*typedValue](ref)
	if gotValue != value {
		t.Fatalf("value mismatch: %#v", gotValue)
	}
	gotContract := MustGetByType[typedContract](ref)
	if gotContract.Value() != "ok" {
		t.Fatalf("contract mismatch: %s", gotContract.Value())
	}
}
