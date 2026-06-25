package compiler

import (
	"errors"
	"reflect"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/di"
	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	corescanner "github.com/sao-lang/lania-g/kernel/v3/scanner"
)

type ownerTestController struct{ name string }
type ownerTestResolver struct{ name string }

type ownerTestModuleA struct{ *module.BaseModule }
type ownerTestModuleB struct{ *module.BaseModule }

// 这个用例验证：当同一 controller 类型在多个模块中出现时，
// BuildSnapshotOwnerIndex 仍然可以借助实例指针精确定位 owner。
func TestBuildSnapshotOwnerIndex_ResolvesAmbiguousControllerByPointer(t *testing.T) {
	ctrlA := &ownerTestController{name: "a"}
	ctrlB := &ownerTestController{name: "b"}
	modA := &ownerTestModuleA{BaseModule: module.NewBaseModule(&module.ModuleMetadata{Controllers: []interface{}{ctrlA}})}
	modB := &ownerTestModuleB{BaseModule: module.NewBaseModule(&module.ModuleMetadata{Controllers: []interface{}{ctrlB}})}
	root := module.CreateModule([]module.Module{modA, modB}, nil, nil, nil, nil)

	index := BuildSnapshotOwnerIndex(corescanner.BuildSnapshot(module.NewModuleRef(root)), SnapshotOwnerOptions{Controllers: true})

	resolvedA := index.Resolve(ctrlA)
	if resolvedA.Status != OwnerResolutionResolved {
		t.Fatalf("ctrlA status = %v, want resolved", resolvedA.Status)
	}
	if resolvedA.Owner.ModuleType.String() != "*compiler.ownerTestModuleA" {
		t.Fatalf("ctrlA owner = %s", resolvedA.Owner.ModuleType)
	}

	resolvedB := index.Resolve(ctrlB)
	if resolvedB.Status != OwnerResolutionResolved {
		t.Fatalf("ctrlB status = %v, want resolved", resolvedB.Status)
	}
	if resolvedB.Owner.ModuleType.String() != "*compiler.ownerTestModuleB" {
		t.Fatalf("ctrlB owner = %s", resolvedB.Owner.ModuleType)
	}

	ambiguous := index.Resolve(&ownerTestController{name: "new"})
	if ambiguous.Status != OwnerResolutionAmbiguous {
		t.Fatalf("new controller status = %v, want ambiguous", ambiguous.Status)
	}
}

// 这个用例验证：BuildSnapshotOwnerIndex 可以同时收集 controllers 与 resolvers，
// 并正确区分 resolved / missing 两类结果。
func TestBuildSnapshotOwnerIndex_CombinesControllersAndResolvers(t *testing.T) {
	ctrl := &ownerTestController{name: "ctrl"}
	resolver := &ownerTestResolver{name: "resolver"}
	modA := &ownerTestModuleA{BaseModule: module.NewBaseModule(&module.ModuleMetadata{
		Controllers: []interface{}{ctrl},
		Resolvers:   []interface{}{resolver},
	})}
	root := module.CreateModule([]module.Module{modA}, nil, nil, nil, nil)

	index := BuildSnapshotOwnerIndex(corescanner.BuildSnapshot(module.NewModuleRef(root)), SnapshotOwnerOptions{
		Controllers: true,
		Resolvers:   true,
	})

	ctrlResolved := index.Resolve(ctrl)
	if ctrlResolved.Status != OwnerResolutionResolved {
		t.Fatalf("controller status = %v, want resolved", ctrlResolved.Status)
	}
	resolverResolved := index.Resolve(resolver)
	if resolverResolved.Status != OwnerResolutionResolved {
		t.Fatalf("resolver status = %v, want resolved", resolverResolved.Status)
	}
	if missing := index.Resolve(nil); missing.Status != OwnerResolutionMissing {
		t.Fatalf("nil status = %v, want missing", missing.Status)
	}
}

func TestBuildSnapshotOwnerIndex_ResolvesValueProviderByPointer(t *testing.T) {
	svcA := &ownerTestController{name: "svc-a"}
	svcB := &ownerTestController{name: "svc-b"}
	providerA, _ := di.ProviderFromInstanceWithToken(reflect.TypeFor[*ownerTestController](), svcA, di.Singleton)
	providerB, _ := di.ProviderFromInstanceWithToken(reflect.TypeFor[*ownerTestController](), svcB, di.Singleton)
	modA := &ownerTestModuleA{BaseModule: module.NewBaseModule(&module.ModuleMetadata{Providers: []*di.Provider{providerA}})}
	modB := &ownerTestModuleB{BaseModule: module.NewBaseModule(&module.ModuleMetadata{Providers: []*di.Provider{providerB}})}
	root := module.CreateModule([]module.Module{modA, modB}, nil, nil, nil, nil)

	index := BuildSnapshotOwnerIndex(corescanner.BuildSnapshot(module.NewModuleRef(root)), SnapshotOwnerOptions{Providers: true})

	resolvedA := index.Resolve(svcA)
	if resolvedA.Status != OwnerResolutionResolved {
		t.Fatalf("svcA status = %v, want resolved", resolvedA.Status)
	}
	if resolvedA.Owner.ModuleType.String() != "*compiler.ownerTestModuleA" {
		t.Fatalf("svcA owner = %s", resolvedA.Owner.ModuleType)
	}

	resolvedB := index.Resolve(svcB)
	if resolvedB.Status != OwnerResolutionResolved {
		t.Fatalf("svcB status = %v, want resolved", resolvedB.Status)
	}
	if resolvedB.Owner.ModuleType.String() != "*compiler.ownerTestModuleB" {
		t.Fatalf("svcB owner = %s", resolvedB.Owner.ModuleType)
	}

	ambiguous := index.Resolve(&ownerTestController{name: "new"})
	if ambiguous.Status != OwnerResolutionAmbiguous {
		t.Fatalf("new provider-backed receiver status = %v, want ambiguous", ambiguous.Status)
	}
}

func TestNewOwnerResolutionError_ProvidesUnifiedMeta(t *testing.T) {
	err := NewOwnerResolutionError(
		"http",
		"http route GET /users/:id",
		OwnerKindReceiver,
		reflect.TypeOf(&ownerTestController{}),
		OwnerResolutionAmbiguous,
		[]ModuleOwner{{ModuleKey: "module.A"}, {ModuleKey: "module.B"}},
		map[string]any{"method": "GET", "path": "/users/:id"},
		"",
	)

	var kernelErr *kerrors.KernelError
	if !errors.As(err, &kernelErr) {
		t.Fatalf("expected KernelError, got %T", err)
	}
	if kernelErr.Kind != kerrors.KindDI {
		t.Fatalf("kind = %v, want %v", kernelErr.Kind, kerrors.KindDI)
	}
	if kernelErr.Meta["ownerKind"] != OwnerKindReceiver {
		t.Fatalf("ownerKind = %v", kernelErr.Meta["ownerKind"])
	}
	if kernelErr.Meta["ownerStatus"] != "ambiguous" {
		t.Fatalf("ownerStatus = %v", kernelErr.Meta["ownerStatus"])
	}
	candidates, ok := kernelErr.Meta["ownerCandidates"].([]string)
	if !ok || len(candidates) != 2 {
		t.Fatalf("ownerCandidates = %#v", kernelErr.Meta["ownerCandidates"])
	}
}
