// modgen 以 modules.json 为准，校验或修正各模块 go.mod 的内部依赖。
//
//	go run ./tools/cmd/modgen -check
//	go run ./tools/cmd/modgen -write
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sao-lang/lania-g/tools/modgen"
)

func main() {
	check := flag.Bool("check", false, "verify go.mod files against modules.json")
	write := flag.Bool("write", false, "rewrite internal require/replace from modules.json")
	dir := flag.String("dir", ".", "repository root")
	flag.Parse()

	if !*check && !*write {
		*check = true
	}

	root, err := filepath.Abs(*dir)
	if err != nil {
		fail(err)
	}
	manifest, err := modgen.Load(root)
	if err != nil {
		fail(err)
	}

	if *write {
		if err := modgen.Write(root, manifest); err != nil {
			fail(err)
		}
		fmt.Printf("rewrote go.mod files for %d modules\n", len(manifest.Modules))
	}

	issues, err := modgen.Check(root, manifest)
	if err != nil {
		fail(err)
	}
	if len(issues) == 0 {
		fmt.Printf("modules ok (%d modules)\n", len(manifest.Modules))
		return
	}
	for _, issue := range issues {
		fmt.Fprintln(os.Stderr, "  "+issue.String())
	}
	fail(fmt.Errorf("%d module inconsistencies found; run `go run ./tools/cmd/modgen -write`", len(issues)))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
