// doccheck 校验文档代码块的语法与框架符号引用。
//
//	go run ./tools/cmd/doccheck
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sao-lang/lania-g/tools/doccheck"
)

func main() {
	root := flag.String("dir", ".", "repository root")
	docsDir := flag.String("docs", "website/content/docs", "docs directory relative to root")
	flag.Parse()

	abs, err := filepath.Abs(*root)
	if err != nil {
		fail(err)
	}

	opts := doccheck.DefaultOptions(abs)
	opts.DocsDir = *docsDir
	findings, err := doccheck.Run(opts)
	if err != nil {
		fail(err)
	}
	if len(findings) == 0 {
		fmt.Println("docs ok")
		return
	}
	for _, finding := range findings {
		fmt.Fprintln(os.Stderr, "  "+finding.String())
	}
	fail(fmt.Errorf("%d doc problem(s) found", len(findings)))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
