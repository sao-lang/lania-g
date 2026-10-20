// apicheck 校验（或更新）仓库的导出 API 快照。
//
//	go run ./tools/cmd/apicheck          # 校验，有差异时退出码 1
//	go run ./tools/cmd/apicheck -update  # 更新 api/api.txt
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sao-lang/lania-g/tools/apicheck"
)

func main() {
	update := flag.Bool("update", false, "update the snapshot instead of verifying")
	dir := flag.String("dir", ".", "repository root (defaults to current directory)")
	flag.Parse()

	root, err := filepath.Abs(*dir)
	if err != nil {
		fail(err)
	}

	snapshot, err := apicheck.Extract(apicheck.DefaultOptions(root))
	if err != nil {
		fail(err)
	}
	path := apicheck.FilePath(root)

	if *update {
		if err := apicheck.Write(path, snapshot); err != nil {
			fail(err)
		}
		fmt.Printf("updated %s (%d packages)\n", path, len(snapshot))
		return
	}

	current, err := os.ReadFile(path)
	if err != nil {
		fail(fmt.Errorf("read snapshot (run with -update first): %w", err))
	}
	if string(current) == apicheck.Render(snapshot) {
		fmt.Printf("api snapshot ok (%d packages)\n", len(snapshot))
		return
	}
	fail(fmt.Errorf("exported API changed; review the diff and run `go run ./tools/cmd/apicheck -update`"))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
