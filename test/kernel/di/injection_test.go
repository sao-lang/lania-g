package di_test

// 测试位于仓库根目录 test/，通过 dot-import 访问被测包。
import . "github.com/sao-lang/lania-g/kernel/v3/di"
import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type injectableStore struct{ Name string }

type injectableLogger struct{}

type hostWithDeps struct {
	Store  *injectableStore
	Logger *injectableLogger
	Extra  string
}

// hostWithWrongPointer 的字段少了指针，用于验证"类型写错"时的候选提示。
type hostWithWrongPointer struct {
	Store injectableStore
}

type hostWithOptional struct {
	Store *injectableStore
	Miss  *injectableLogger `di:"optional"`
}

type hostWithSkip struct {
	Store *injectableStore
	Miss  *injectableLogger `di:"-"`
}

func newContainerWith(t *testing.T, register bool) *Container {
	t.Helper()
	c := NewContainer()
	if register {
		c.RegisterValue(reflect.TypeFor[*injectableStore](), &injectableStore{Name: "s"})
	}
	return c
}

func TestInjectionReportRecordsSuccess(t *testing.T) {
	c := newContainerWith(t, true)
	c.RegisterClass(reflect.TypeFor[*hostWithDeps](), reflect.TypeFor[hostWithDeps](), Transient)

	if _, err := c.Get(reflect.TypeFor[*hostWithDeps]()); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	var injected, missing int
	for _, record := range c.InjectionReport() {
		switch {
		case record.Status == InjectionInjected && record.Field == "Store":
			injected++
		case record.Status == InjectionMissingProvider && record.Field == "Logger":
			missing++
		}
	}
	if injected != 1 {
		t.Fatalf("Store should be injected, records=%v", c.InjectionReport())
	}
	if missing != 1 {
		t.Fatalf("Logger should be missing, records=%v", c.InjectionReport())
	}
}

func TestInjectionProblemReportsCandidates(t *testing.T) {
	c := NewContainer()
	// 注册指针类型，但宿主字段写成值类型——这是最常见的"类型写错"。
	c.RegisterValue(reflect.TypeFor[*injectableStore](), &injectableStore{})

	c.RegisterClass(reflect.TypeFor[*hostWithWrongPointer](), reflect.TypeFor[hostWithWrongPointer](), Transient)
	if _, err := c.Get(reflect.TypeFor[*hostWithWrongPointer]()); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	problems := c.InjectionProblems()
	if len(problems) == 0 {
		t.Fatalf("expected at least one problem")
	}
	found := false
	for _, problem := range problems {
		if problem.Field == "Store" {
			found = true
			if len(problem.Candidates) == 0 {
				t.Fatalf("Store should hint nearest tokens, got %v", problem)
			}
		}
	}
	if !found {
		t.Fatalf("Store problem not reported: %v", problems)
	}
}

func TestOptionalFieldIsNotAProblem(t *testing.T) {
	c := NewContainer()
	c.RegisterValue(reflect.TypeFor[*injectableStore](), &injectableStore{})
	c.RegisterClass(reflect.TypeFor[*hostWithOptional](), reflect.TypeFor[hostWithOptional](), Transient)
	if _, err := c.Get(reflect.TypeFor[*hostWithOptional]()); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	for _, record := range c.InjectionReport() {
		if record.Field == "Miss" && record.Status == InjectionMissingProvider {
			t.Fatalf("optional field must not be reported as missing: %v", record)
		}
		if record.Field == "Miss" && record.Status != InjectionOptional {
			t.Fatalf("optional field status = %s", record.Status)
		}
	}
	if problems := c.InjectionProblems(); len(problems) != 0 {
		t.Fatalf("optional field produced problems: %v", problems)
	}
}

func TestSkippedFieldRecorded(t *testing.T) {
	c := NewContainer()
	c.RegisterClass(reflect.TypeFor[*hostWithSkip](), reflect.TypeFor[hostWithSkip](), Transient)
	if _, err := c.Get(reflect.TypeFor[*hostWithSkip]()); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	for _, record := range c.InjectionReport() {
		if record.Field == "Miss" && record.Status != InjectionSkipped {
			t.Fatalf("di:\"-\" field status = %s", record.Status)
		}
	}
}

func TestStrictInjectionFailsOnMissing(t *testing.T) {
	c := newContainerWith(t, true)
	c.SetStrictInjection(true)
	c.RegisterClass(reflect.TypeFor[*hostWithDeps](), reflect.TypeFor[hostWithDeps](), Transient)

	_, err := c.Get(reflect.TypeFor[*hostWithDeps]())
	if err == nil {
		t.Fatalf("expected strict injection error")
	}
	message := err.Error()
	for _, want := range []string{"hostWithDeps.Logger", "provider not found", "optional"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error missing %q: %s", want, message)
		}
	}
}

func TestStrictInjectionAllowsOptional(t *testing.T) {
	c := NewContainer()
	c.SetStrictInjection(true)
	c.RegisterValue(reflect.TypeFor[*injectableStore](), &injectableStore{})
	c.RegisterClass(reflect.TypeFor[*hostWithOptional](), reflect.TypeFor[hostWithOptional](), Transient)

	if _, err := c.Get(reflect.TypeFor[*hostWithOptional]()); err != nil {
		t.Fatalf("optional field must not fail strict mode: %v", err)
	}
}

func TestNewChildInheritsStrictMode(t *testing.T) {
	parent := NewContainer()
	parent.SetStrictInjection(true)
	child := parent.NewChild()

	if !child.StrictInjection() {
		t.Fatalf("child should inherit strict mode")
	}

	child.RegisterClass(reflect.TypeFor[*hostWithDeps](), reflect.TypeFor[hostWithDeps](), Transient)
	child.RegisterValue(reflect.TypeFor[*injectableStore](), &injectableStore{})

	_, err := child.Get(reflect.TypeFor[*hostWithDeps]())
	if err == nil {
		t.Fatalf("expected strict error in child container")
	}
	var target interface{ Error() string }
	if !errors.As(err, &target) {
		t.Fatalf("unexpected error type: %T", err)
	}
}

func TestInjectionAuditCanBeDisabled(t *testing.T) {
	c := NewContainer()
	c.SetInjectionAudit(false)
	c.RegisterClass(reflect.TypeFor[*hostWithDeps](), reflect.TypeFor[hostWithDeps](), Transient)
	if _, err := c.Get(reflect.TypeFor[*hostWithDeps]()); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if records := c.InjectionReport(); len(records) != 0 {
		t.Fatalf("audit disabled but got %d records", len(records))
	}
}
