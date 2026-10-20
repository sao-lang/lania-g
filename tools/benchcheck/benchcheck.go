// Package benchcheck 解析 `go test -bench` 输出并在两次运行之间做回归比较。
//
// 它刻意不依赖 golang.org/x/perf/benchstat：
//
//   - 只需要"变慢了多少"这一个判断
//   - 避免给工具链引入额外依赖
//
// 用法：
//
//	go test ./test/bench -bench . -benchmem -count=6 > new.txt
//	go run ./tools/cmd/benchcheck -baseline test/bench/baseline.txt -current new.txt
package benchcheck

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Result 是一个 benchmark 的聚合结果（多轮 `-count=N` 取中位数）。
type Result struct {
	Name      string
	NsPerOp   float64
	BytesPerOp float64
	AllocsPerOp float64
	Samples   int
}

// linePattern 匹配 `BenchmarkXxx-16  1000  1234 ns/op  567 B/op  8 allocs/op`。
var linePattern = regexp.MustCompile(
	`^(Benchmark\S+?)(?:-\d+)?\s+(\d+)\s+([\d.]+)\s+ns/op(?:\s+([\d.]+)\s+B/op)?(?:\s+([\d.]+)\s+allocs/op)?`)

// Parse 读取 benchmark 输出，按名字聚合（取中位数）。
func Parse(path string) (map[string]Result, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	samples := make(map[string][]Result)
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "Benchmark") {
			continue
		}
		match := linePattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		ns, _ := strconv.ParseFloat(match[3], 64)
		bytesPerOp, _ := strconv.ParseFloat(match[4], 64)
		allocs, _ := strconv.ParseFloat(match[5], 64)
		samples[match[1]] = append(samples[match[1]], Result{
			Name:        match[1],
			NsPerOp:     ns,
			BytesPerOp:  bytesPerOp,
			AllocsPerOp: allocs,
		})
	}

	out := make(map[string]Result, len(samples))
	for name, list := range samples {
		out[name] = median(name, list)
	}
	return out, nil
}

func median(name string, list []Result) Result {
	if len(list) == 0 {
		return Result{Name: name}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].NsPerOp < list[j].NsPerOp })
	mid := list[len(list)/2]
	mid.Samples = len(list)
	return mid
}

// Options 控制比较的容差。
type Options struct {
	// Threshold 是允许的相对回退（0.1 表示 10%）。
	Threshold float64
	// AllocTolerance 是允许的 allocs/op 绝对增量。
	AllocTolerance float64
}

// Regression 描述一项回退。
type Regression struct {
	Name    string
	Metric  string
	Baseline float64
	Current  float64
	Delta    float64
}

// String 以单行文本描述回退。
func (r Regression) String() string {
	return fmt.Sprintf("%-32s %-12s baseline=%.1f current=%.1f (%+.1f%%)",
		r.Name, r.Metric, r.Baseline, r.Current, r.Delta*100)
}

// Compare 比较基线与当前结果。
//
// 只检查两边都存在的 benchmark；新增或删除的 benchmark 不视为回退。
func Compare(baseline, current map[string]Result, opts Options) []Regression {
	if opts.Threshold <= 0 {
		opts.Threshold = 0.1
	}
	var regressions []Regression
	for name, base := range baseline {
		now, ok := current[name]
		if !ok || base.NsPerOp <= 0 {
			continue
		}
		delta := (now.NsPerOp - base.NsPerOp) / base.NsPerOp
		if delta > opts.Threshold {
			regressions = append(regressions, Regression{
				Name: name, Metric: "ns/op",
				Baseline: base.NsPerOp, Current: now.NsPerOp, Delta: delta,
			})
		}
		if base.AllocsPerOp > 0 && now.AllocsPerOp-base.AllocsPerOp > opts.AllocTolerance {
			regressions = append(regressions, Regression{
				Name: name, Metric: "allocs/op",
				Baseline: base.AllocsPerOp, Current: now.AllocsPerOp,
				Delta: (now.AllocsPerOp - base.AllocsPerOp) / base.AllocsPerOp,
			})
		}
	}
	sort.Slice(regressions, func(i, j int) bool {
		if regressions[i].Name != regressions[j].Name {
			return regressions[i].Name < regressions[j].Name
		}
		return regressions[i].Metric < regressions[j].Metric
	})
	return regressions
}
