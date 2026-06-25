package integration

import (
	"reflect"
	"testing"
)

type testWrapper[T any] struct {
	Value T
}

type testRef[N any] struct {
	Value int
	_     *N
}

type namedMarker struct{}

func (namedMarker) Name() string { return "named" }

type fallbackMarker struct{}

func TestMatchNamedWrapper(t *testing.T) {
	match := MatchNamedWrapper(reflect.TypeOf(testWrapper[int]{}).PkgPath(), "testWrapper")

	desc, ok := match(reflect.TypeOf(testWrapper[int]{}))
	if !ok {
		t.Fatalf("expected wrapper match")
	}
	if desc.Kind != "testWrapper" {
		t.Fatalf("kind=%s", desc.Kind)
	}
	if desc.InnerType != reflect.TypeOf(int(0)) {
		t.Fatalf("inner=%s", desc.InnerType)
	}
}

func TestResolveMarkerName(t *testing.T) {
	got := ResolveMarkerName(reflect.TypeOf(testRef[namedMarker]{}), "default", func(marker any) (string, bool) {
		namer, ok := marker.(interface{ Name() string })
		if !ok {
			return "", false
		}
		return namer.Name(), true
	})
	if got != "named" {
		t.Fatalf("got=%s", got)
	}

	got = ResolveMarkerName(reflect.TypeOf(testRef[fallbackMarker]{}), "default", func(marker any) (string, bool) {
		return "", false
	})
	if got != "fallbackmarker" {
		t.Fatalf("fallback got=%s", got)
	}
}

func TestWrapFirstField(t *testing.T) {
	value, err := WrapFirstField(reflect.TypeOf(testWrapper[int]{}), reflect.ValueOf(42))
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}
	wrapped := value.(testWrapper[int])
	if wrapped.Value != 42 {
		t.Fatalf("value=%d", wrapped.Value)
	}
}
