package di_test

// 锁定字段注入的「审计记录」语义：优化注入计划缓存后，每个字段的状态仍必须如实上报。
// 循环依赖检测改用池化 map 后，也不能出现"上次解析残留导致的误报"。

import (
	"reflect"
	"sync"
	"testing"

	. "github.com/sao-lang/lania-g/kernel/v3/di"
)

type planDep struct{ N int }

type planTarget struct {
	Injected   *planDep     // 零值 + 有 provider → injected
	Unexported *planDep     // 未导出 → unexported
	Skipped    *planDep     `di:"-"`        // 显式跳过
	Optional   *planMissing `di:"optional"` // 缺失但允许
}

type planMissing struct{}

func registerPlanProviders(c *Container) {
	c.RegisterValue(TokenFor[*planDep](), &planDep{N: 1})
}

func TestInjectionReportRecordsEveryFieldStatus(t *testing.T) {
	c := NewContainer()
	registerPlanProviders(c)
	c.RegisterClass(TokenFor[*planTarget](), reflect.TypeFor[planTarget](), Transient)

	if _, err := c.Get(TokenFor[*planTarget]()); err != nil {
		t.Fatalf("resolve target: %v", err)
	}

	statuses := map[string]InjectionStatus{}
	for _, record := range c.InjectionReport() {
		if record.TargetType != reflect.TypeFor[planTarget]() {
			continue
		}
		statuses[record.Field] = record.Status
	}

	want := map[string]InjectionStatus{
		"Injected": InjectionInjected,
		"Skipped":  InjectionSkipped,
		"Optional": InjectionOptional,
	}
	for field, expected := range want {
		if got, ok := statuses[field]; !ok || got != expected {
			t.Fatalf("%s 状态 = %q（存在=%v），want %q（全部记录：%v）", field, got, ok, expected, statuses)
		}
	}
}

func TestInjectionRunsForEveryConstruction(t *testing.T) {
	c := NewContainer()
	registerPlanProviders(c)
	c.RegisterClass(TokenFor[*planRepeated](), reflect.TypeFor[planRepeated](), Transient)

	const times = 20
	for i := 0; i < times; i++ {
		value, err := c.Get(TokenFor[*planRepeated]())
		if err != nil {
			t.Fatalf("resolve #%d: %v", i, err)
		}
		if value.(*planRepeated).Dep == nil {
			t.Fatalf("resolve #%d: 依赖未注入", i)
		}
	}

	var count int
	for _, record := range c.InjectionReport() {
		if record.TargetType == reflect.TypeFor[planRepeated]() && record.Status == InjectionInjected {
			count++
		}
	}
	if count != times {
		t.Fatalf("注入记录 = %d 条，want %d（每次构造都必须如实记录）", count, times)
	}
}

type planRepeated struct{ Dep *planDep }

func TestInjectionReportEmptyWhenAuditDisabled(t *testing.T) {
	c := NewContainer()
	c.SetInjectionAudit(false)
	registerPlanProviders(c)
	c.RegisterClass(TokenFor[*planRepeated](), reflect.TypeFor[planRepeated](), Transient)

	if _, err := c.Get(TokenFor[*planRepeated]()); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if records := c.InjectionReport(); len(records) != 0 {
		t.Fatalf("审计关闭后仍记录 %d 条", len(records))
	}
}

type cycleA struct{ B *cycleB }

type cycleB struct{ A *cycleA }

func TestCircularDependencyDetected(t *testing.T) {
	c := NewContainer()
	c.SetStrictInjection(true)
	c.RegisterClass(TokenFor[*cycleA](), reflect.TypeFor[cycleA](), Transient)
	c.RegisterClass(TokenFor[*cycleB](), reflect.TypeFor[cycleB](), Transient)

	if _, err := c.Get(TokenFor[*cycleA]()); err == nil {
		t.Fatalf("expected circular dependency error")
	}
}

// 环检测失败后必须清理干净：池化/复用检测状态时最容易出现的回归是"上一次的残留导致下一次误报"。
func TestGetSucceedsAfterCircularDependencyFailure(t *testing.T) {
	c := NewContainer()
	c.SetStrictInjection(true)
	c.RegisterClass(TokenFor[*cycleA](), reflect.TypeFor[cycleA](), Transient)
	c.RegisterClass(TokenFor[*cycleB](), reflect.TypeFor[cycleB](), Transient)
	registerPlanProviders(c)

	if _, err := c.Get(TokenFor[*cycleA]()); err == nil {
		t.Fatalf("expected circular dependency error")
	}

	value, err := c.Get(TokenFor[*planDep]())
	if err != nil {
		t.Fatalf("环检测失败后的正常解析失败了：%v", err)
	}
	if value.(*planDep).N != 1 {
		t.Fatalf("N = %d, want 1", value.(*planDep).N)
	}
}

func TestConcurrentResolveIsConsistent(t *testing.T) {
	c := NewContainer()
	registerPlanProviders(c)
	c.RegisterClass(TokenFor[*planRepeated](), reflect.TypeFor[planRepeated](), Transient)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				value, err := c.Get(TokenFor[*planRepeated]())
				if err != nil {
					t.Errorf("resolve: %v", err)
					return
				}
				if value.(*planRepeated).Dep == nil {
					t.Errorf("依赖未注入")
					return
				}
			}
		}()
	}
	wg.Wait()
}
