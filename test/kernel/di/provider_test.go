package di_test

// 测试位于仓库根目录 test/，通过 dot-import 访问被测包。
import . "github.com/sao-lang/lania-g/kernel/v3/di"
import (
	"reflect"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/metadata"
)

type valueProviderStore struct {
	Name string
}

type valueProviderContract interface {
	Label() string
}

func (s *valueProviderStore) Label() string { return s.Name }

func TestTokenFor(t *testing.T) {
	if got, want := TokenFor[*valueProviderStore](), reflect.TypeFor[*valueProviderStore](); got != want {
		t.Fatalf("TokenFor = %v, want %v", got, want)
	}
	if got, want := TokenFor[valueProviderContract](), reflect.TypeFor[valueProviderContract](); got != want {
		t.Fatalf("interface token = %v, want %v", got, want)
	}
}

func TestProviderFromValueUsesGenericToken(t *testing.T) {
	store := &valueProviderStore{Name: "main"}

	provider, err := ProviderFromValue[*valueProviderStore](store, Singleton)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if provider.Type != ProviderTypeValue {
		t.Fatalf("type = %s, want value", provider.Type)
	}
	if provider.Token != reflect.TypeFor[*valueProviderStore]() {
		t.Fatalf("token = %v", provider.Token)
	}
	if provider.UseValue != store {
		t.Fatalf("use value mismatch")
	}
	if provider.Scope != Singleton {
		t.Fatalf("scope = %s", provider.Scope)
	}
}

// 接口类型作为泛型参数时，token 应是接口本身，而不是实现的动态类型。
func TestProviderFromValueUsesInterfaceToken(t *testing.T) {
	var impl valueProviderContract = &valueProviderStore{Name: "impl"}

	provider, err := ProviderFromValue[valueProviderContract](impl, Singleton)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if provider.Token != reflect.TypeFor[valueProviderContract]() {
		t.Fatalf("token = %v, want interface type", provider.Token)
	}

	container := NewContainer()
	container.RegisterValue(provider.Token, provider.UseValue)

	resolved, err := container.Get(reflect.TypeFor[valueProviderContract]())
	if err != nil {
		t.Fatalf("resolve by interface: %v", err)
	}
	got, ok := resolved.(valueProviderContract)
	if !ok || got.Label() != "impl" {
		t.Fatalf("resolved = %#v", resolved)
	}
}

func TestProviderFromValueMatchesWithTokenVariant(t *testing.T) {
	store := &valueProviderStore{Name: "same"}

	viaValue, err := ProviderFromValue[*valueProviderStore](store, metadata.ScopeSingleton)
	if err != nil {
		t.Fatalf("ProviderFromValue: %v", err)
	}
	viaToken, err := ProviderFromInstanceWithToken(TokenFor[*valueProviderStore](), store, metadata.ScopeSingleton)
	if err != nil {
		t.Fatalf("ProviderFromInstanceWithToken: %v", err)
	}

	if viaValue.Token != viaToken.Token || viaValue.Type != viaToken.Type || viaValue.Scope != viaToken.Scope {
		t.Fatalf("providers differ: %+v vs %+v", viaValue, viaToken)
	}
	if viaValue.UseValue != viaToken.UseValue {
		t.Fatalf("use value differs")
	}
}
