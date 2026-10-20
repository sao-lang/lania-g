package bench_test

import (
	"os"
	"testing"

	"github.com/sao-lang/lania-g/tools/benchcheck"
)

// TestBaselineIsParsable 保证基线文件格式合法（避免门禁因为格式问题失效）。
//
// 真正的性能比对是一个独立命令，因为它需要多轮采样：
//
//	go test ./test/bench -bench . -benchmem -count=6 > new.txt
//	go run ./tools/cmd/benchcheck -current new.txt
func TestBaselineIsParsable(t *testing.T) {
	results, err := benchcheck.Parse("baseline.txt")
	if err != nil {
		t.Fatalf("parse baseline: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("baseline.txt has no benchmark results")
	}
	for _, name := range []string{
		"BenchmarkHTTP_StaticJSON",
		"BenchmarkHTTP_AOPDepth10",
		"BenchmarkHTTP_BindValidate",
		"BenchmarkDI_ResolveNested",
	} {
		if _, ok := results[name]; !ok {
			t.Fatalf("baseline missing %s", name)
		}
	}
}

// TestBaselineFileExists 保证基线文件被提交进仓库。
func TestBaselineFileExists(t *testing.T) {
	if _, err := os.Stat("baseline.txt"); err != nil {
		t.Fatalf("baseline.txt missing: %v", err)
	}
}
