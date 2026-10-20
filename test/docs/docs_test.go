package docs_test

import (
	"path/filepath"
	"testing"

	"github.com/sao-lang/lania-g/tools/doccheck"
)

// TestDocsMatchSource 校验文档里的框架符号真实存在，且标记为 `go check` 的代码块语法正确。
//
// 它拦住的是最难人工发现的一类问题：文档写了一个不存在的 API。
func TestDocsMatchSource(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	findings, err := doccheck.Run(doccheck.DefaultOptions(root))
	if err != nil {
		t.Fatalf("doccheck: %v", err)
	}
	if len(findings) == 0 {
		return
	}
	for _, finding := range findings {
		t.Errorf("%s", finding.String())
	}
	t.Fatalf("%d doc problem(s); 修复文档或运行 `go run ./tools/cmd/doccheck` 查看详情", len(findings))
}
