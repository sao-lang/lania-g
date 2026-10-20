package di_test

// 测试位于仓库根目录 test/，通过 dot-import 访问被测包。
import . "github.com/sao-lang/lania-g/kernel/v3/di"
import "reflect"
import "testing"

type typedContainerValue struct {
	Name string
}

type typedContainerContract interface {
	Value() string
}

type typedContainerImpl struct{}

func (typedContainerImpl) Value() string { return "ok" }

type typedContainerInjectableValue struct {
	Dep *typedContainerValue
}

func TestGet_ValueProviderWithStructValueDoesNotPanic(t *testing.T) {
	container := NewContainer()
	dep := &typedContainerValue{Name: "dep"}
	container.RegisterValue(reflect.TypeFor[*typedContainerValue](), dep)
	container.RegisterValue(reflect.TypeFor[typedContainerInjectableValue](), typedContainerInjectableValue{})

	got, err := GetByType[typedContainerInjectableValue](container)
	if err != nil {
		t.Fatalf("get value: %v", err)
	}
	// 值类型不可寻址，注入不可见，只要求不 panic 且返回注册的零值副本。
	if got.Dep != nil {
		t.Fatalf("expected zero value copy, got %#v", got.Dep)
	}
}

func TestMustGetByType(t *testing.T) {
	container := NewContainer()
	value := &typedContainerValue{Name: "demo"}
	contract := typedContainerContract(typedContainerImpl{})

	container.RegisterValue(reflect.TypeFor[*typedContainerValue](), value)
	container.RegisterValue(reflect.TypeFor[typedContainerContract](), contract)

	gotValue := MustGetByType[*typedContainerValue](container)
	if gotValue != value {
		t.Fatalf("value mismatch: %#v", gotValue)
	}

	gotContract := MustGetByType[typedContainerContract](container)
	if gotContract.Value() != "ok" {
		t.Fatalf("contract mismatch: %s", gotContract.Value())
	}
}
