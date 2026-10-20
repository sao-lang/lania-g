package di_test

// 锁定「请求级子容器」的作用域语义：
// 优化子容器的分配方式时，request scope 实例绝不能跨请求复用。

import (
	"reflect"
	"testing"

	. "github.com/sao-lang/lania-g/kernel/v3/di"
)

type scopedThing struct{ N int }

type singletonThing struct{ N int }

func TestNewChildIsolatesRequestScopedInstances(t *testing.T) {
	root := NewContainer()
	root.RegisterClass(TokenFor[*scopedThing](), reflect.TypeFor[scopedThing](), Request)

	first, err := root.NewChild().Get(TokenFor[*scopedThing]())
	if err != nil {
		t.Fatalf("resolve in first child: %v", err)
	}
	second, err := root.NewChild().Get(TokenFor[*scopedThing]())
	if err != nil {
		t.Fatalf("resolve in second child: %v", err)
	}

	if first == second {
		t.Fatalf("request-scoped instance shared across children")
	}
}

func TestNewChildReusesSingletonFromParent(t *testing.T) {
	root := NewContainer()
	root.RegisterClass(TokenFor[*singletonThing](), reflect.TypeFor[singletonThing](), Singleton)

	if _, err := root.Get(TokenFor[*singletonThing]()); err != nil {
		t.Fatalf("resolve in root: %v", err)
	}
	child := root.NewChild()

	fromRoot, err := root.Get(TokenFor[*singletonThing]())
	if err != nil {
		t.Fatalf("resolve in root: %v", err)
	}
	fromChild, err := child.Get(TokenFor[*singletonThing]())
	if err != nil {
		t.Fatalf("resolve in child: %v", err)
	}

	if fromRoot != fromChild {
		t.Fatalf("singleton resolved differently in child")
	}
}

func TestNewChildSeesParentProviders(t *testing.T) {
	root := NewContainer()
	root.RegisterValue(TokenFor[*singletonThing](), &singletonThing{N: 7})

	child := root.NewChild()
	value, err := child.Get(TokenFor[*singletonThing]())
	if err != nil {
		t.Fatalf("resolve in child: %v", err)
	}
	if got := value.(*singletonThing).N; got != 7 {
		t.Fatalf("N = %d, want 7", got)
	}
}

func TestChildRegistrationOverridesParent(t *testing.T) {
	root := NewContainer()
	root.RegisterValue(TokenFor[*singletonThing](), &singletonThing{N: 1})

	child := root.NewChild()
	child.RegisterValue(TokenFor[*singletonThing](), &singletonThing{N: 2})

	value, err := child.Get(TokenFor[*singletonThing]())
	if err != nil {
		t.Fatalf("resolve in child: %v", err)
	}
	if got := value.(*singletonThing).N; got != 2 {
		t.Fatalf("N = %d, want 2（child 应覆盖 parent）", got)
	}
}

func TestEmptyChildDoesNotAffectParentResolution(t *testing.T) {
	root := NewContainer()
	root.RegisterClass(TokenFor[*singletonThing](), reflect.TypeFor[singletonThing](), Singleton)

	child := root.NewChild()
	// 子容器上做若干次解析（其中包含未注册 token 的失败解析），不应污染 parent。
	if _, err := child.Get(TokenFor[*singletonThing]()); err != nil {
		t.Fatalf("resolve in child: %v", err)
	}
	if _, err := child.Get("missing-token"); err == nil {
		t.Fatalf("expected error for missing token")
	}

	value, err := root.Get(TokenFor[*singletonThing]())
	if err != nil {
		t.Fatalf("resolve in root: %v", err)
	}
	if value == nil {
		t.Fatalf("nil instance from root")
	}
}
