// benchcheck 比较两次 benchmark 结果，发现性能回退。
//
//	go run ./tools/cmd/benchcheck -baseline test/bench/baseline.txt -current new.txt
//
// 退出码 1 表示存在超过阈值的回退。
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sao-lang/lania-g/tools/benchcheck"
)

func main() {
	baselinePath := flag.String("baseline", "test/bench/baseline.txt", "baseline benchmark output")
	currentPath := flag.String("current", "", "current benchmark output (required)")
	threshold := flag.Float64("threshold", 0.10, "allowed ns/op regression ratio (0.10 = 10%)")
	allocTolerance := flag.Float64("alloc-tolerance", 1, "allowed allocs/op absolute increase")
	write := flag.Bool("write", false, "overwrite the baseline with -current output")
	flag.Parse()

	if *currentPath == "" {
		fail(fmt.Errorf("-current is required"))
	}

	if *write {
		current, err := benchcheck.Parse(*currentPath)
		if err != nil {
			fail(fmt.Errorf("read current: %w", err))
		}
		if len(current) == 0 {
			fail(fmt.Errorf("no benchmark results found in %s", *currentPath))
		}
		if err := copyFile(*currentPath, *baselinePath); err != nil {
			fail(err)
		}
		fmt.Printf("baseline updated: %s (%d benchmarks)\n", *baselinePath, len(current))
		return
	}

	baseline, err := benchcheck.Parse(*baselinePath)
	if err != nil {
		fail(fmt.Errorf("read baseline: %w", err))
	}
	current, err := benchcheck.Parse(*currentPath)
	if err != nil {
		fail(fmt.Errorf("read current: %w", err))
	}
	if len(current) == 0 {
		fail(fmt.Errorf("no benchmark results found in %s", *currentPath))
	}

	regressions := benchcheck.Compare(baseline, current, benchcheck.Options{
		Threshold:      *threshold,
		AllocTolerance: *allocTolerance,
	})
	if len(regressions) == 0 {
		fmt.Printf("benchmark ok (%d benchmarks compared)\n", len(current))
		return
	}

	fmt.Fprintln(os.Stderr, "performance regression detected:")
	for _, regression := range regressions {
		fmt.Fprintln(os.Stderr, "  "+regression.String())
	}
	fmt.Fprintf(os.Stderr, "\n如果回退是有意的，请重新生成基线：\n  go run ./tools/cmd/benchcheck -write %s\n", *baselinePath)
	os.Exit(1)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func copyFile(src, dst string) error {
	content, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	if err := os.WriteFile(dst, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}
