package api_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sao-lang/lania-g/tools/apicheck"
)

// repoRoot 从测试文件所在目录回溯到仓库根（test/api -> ../..）。
func repoRoot(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return abs
}

// TestExportedAPIStable 保证导出符号只按显式评审过的快照变化。
//
// 失败时的正确做法：
//  1. 确认改动是有意的
//  2. 运行 `go run ./tools/cmd/apicheck -update`
//  3. 把 api/api.txt 的 diff 一起提交
func TestExportedAPIStable(t *testing.T) {
	if testing.Short() {
		t.Skip("apicheck type-checks every module (~1min); run without -short")
	}
	root := repoRoot(t)

	snapshot, err := apicheck.Extract(apicheck.DefaultOptions(root))
	if err != nil {
		t.Fatalf("extract api: %v", err)
	}
	if len(snapshot) == 0 {
		t.Fatalf("no packages extracted; check go.work")
	}

	path := apicheck.FilePath(root)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s (run `go run ./tools/cmd/apicheck -update`): %v", path, err)
	}

	if got, want := apicheck.Render(snapshot), string(raw); got != want {
		t.Fatalf("exported API changed, but api/api.txt was not updated.\n\n" +
			"如果这是有意的公开 API 变更：\n" +
			"  go run ./tools/cmd/apicheck -update\n" +
			"并把 api/api.txt 的 diff 一起提交。\n\n" +
			"如果是无意的：说明某个导出符号被误删/改名/改签名了。\n\n" +
			"详见 docs/API-STABILITY.md")
	}
}
