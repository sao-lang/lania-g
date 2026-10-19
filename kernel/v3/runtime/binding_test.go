package runtime

import (
	"errors"
	"reflect"
	"testing"
)

type protocolBindingSample struct {
	Name string
}

func newProtocolResolver(name string, protocol Protocol, value string) BindingRegistration {
	target := reflect.TypeFor[protocolBindingSample]()
	return BindingRegistration{
		Name:             name,
		AllowedProtocols: map[Protocol]bool{protocol: true},
		Match: func(t reflect.Type) (WrapperDescriptor, bool) {
			if t != target {
				return WrapperDescriptor{}, false
			}
			return WrapperDescriptor{Kind: name, WrapperType: t, InnerType: t}, true
		},
		Resolve: func(ctx *HandlerContext, desc WrapperDescriptor) (interface{}, error) {
			return value, nil
		},
	}
}

func resolveSample(br *BindingRegistry, protocol Protocol) (interface{}, error) {
	ctx := NewHandlerContext(protocol)
	return br.Resolve(ctx, nil, reflect.TypeFor[protocolBindingSample](), 0)
}

// 同名参数类型在多协议下必须各自命中本协议的 resolver，
// 而不能因为“后注册优先”被另一个协议的 resolver 抢占。
func TestBindingRegistryResolveByProtocol(t *testing.T) {
	br := NewBindingRegistry()
	br.RegisterBinding(newProtocolResolver("httpAutoStruct", "http", "http-value"))
	br.RegisterBinding(newProtocolResolver("graphqlAutoStruct", "graphql", "graphql-value"))

	got, err := resolveSample(br, "http")
	if err != nil {
		t.Fatalf("resolve http: %v", err)
	}
	if got != "http-value" {
		t.Fatalf("http value = %v, want http-value", got)
	}

	got, err = resolveSample(br, "graphql")
	if err != nil {
		t.Fatalf("resolve graphql: %v", err)
	}
	if got != "graphql-value" {
		t.Fatalf("graphql value = %v, want graphql-value", got)
	}

	// 缓存必须按协议隔离，避免先解析的协议污染后续协议。
	got, err = resolveSample(br, "http")
	if err != nil || got != "http-value" {
		t.Fatalf("cached http resolve = %v, %v", got, err)
	}
}

func TestBindingRegistryResolveKeepsRegistrationPriorityWithinProtocol(t *testing.T) {
	br := NewBindingRegistry()
	br.RegisterBinding(newProtocolResolver("base", "http", "base"))
	br.RegisterBinding(newProtocolResolver("override", "http", "override"))

	got, err := resolveSample(br, "http")
	if err != nil {
		t.Fatalf("resolve http: %v", err)
	}
	if got != "override" {
		t.Fatalf("value = %v, want override (后注册优先)", got)
	}
}

func TestBindingRegistryResolveReportsProtocolMismatch(t *testing.T) {
	br := NewBindingRegistry()
	br.RegisterBinding(newProtocolResolver("httpAutoStruct", "http", "http-value"))

	_, err := resolveSample(br, "graphql")
	if !errors.Is(err, ErrBindingNotSupported) {
		t.Fatalf("err = %v, want ErrBindingNotSupported", err)
	}
}

func TestBindingRegistryResolveReturnsNotFoundWhenNoTypeMatch(t *testing.T) {
	br := NewBindingRegistry()
	ctx := NewHandlerContext("http")

	_, err := br.Resolve(ctx, nil, reflect.TypeFor[struct{ Other string }](), 0)
	if !errors.Is(err, ErrBindingNotFound) {
		t.Fatalf("err = %v, want ErrBindingNotFound", err)
	}
}
