// modrun 在所有 workspace 模块里执行同一个 go 命令。
//
//	go run ./tools/cmd/modrun build
//	go run ./tools/cmd/modrun test -short
//	go run ./tools/cmd/modrun vet
//	go run ./tools/cmd/modrun deps    # 校验依赖边界是否符合 modules.json
//
// 与手工 foreach 相比：
//   - 参数与输出统一，失败时聚合报告
//   - 模块列表来自 modules.json，不会漏掉新模块
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/sao-lang/lania-g/tools/modgen"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	command := os.Args[1]
	extra := os.Args[2:]

	root, err := resolveRoot()
	if err != nil {
		fail(err)
	}
	manifest, err := modgen.Load(root)
	if err != nil {
		fail(err)
	}

	switch command {
	case "deps":
		fail(boundaryCheck(root, manifest))
	case "build", "test", "vet", "tidy":
		fail(runAll(root, manifest, command, extra))
	default:
		usage()
	}
}

// resolveRoot 找到包含 modules.json 的最近祖先目录。
//
// 这样无论从仓库根还是从 tools/ 下执行都能工作。
func resolveRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "modules.json")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("modules.json not found in any parent of the working directory")
		}
		dir = parent
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: modrun <build|test|vet|tidy|deps> [go args...]")
	os.Exit(2)
}

type result struct {
	module string
	output string
	err    error
}

func runAll(root string, manifest *modgen.Manifest, command string, extra []string) error {
	var targets []modgen.Module
	for _, module := range manifest.Modules {
		// 根模块没有包，跳过以免噪音。
		if module.Path == "." {
			continue
		}
		targets = append(targets, module)
	}

	results := make(chan result, len(targets))
	var wait sync.WaitGroup
	for _, module := range targets {
		wait.Add(1)
		go func(module modgen.Module) {
			defer wait.Done()
			dir := filepath.Join(root, filepath.FromSlash(module.Path))
			args := append([]string{command}, extra...)
			// `go mod tidy` 不接受 package patterns；其余命令默认作用于 ./...
			if command != "tidy" {
				args = append(args, "./...")
			}
			cmd := exec.Command("go", args...)
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			results <- result{module: module.Path, output: string(out), err: err}
		}(module)
	}
	wait.Wait()
	close(results)

	var failures []result
	ok := 0
	for item := range results {
		if item.err != nil {
			failures = append(failures, item)
			continue
		}
		ok++
	}
	sort.Slice(failures, func(i, j int) bool { return failures[i].module < failures[j].module })

	if len(failures) == 0 {
		fmt.Printf("go %s: all %d modules ok\n", command, ok)
		return nil
	}
	for _, failure := range failures {
		fmt.Printf("\n=== %s ===\n%s\n", failure.module, strings.TrimRight(failure.output, "\n"))
	}
	return fmt.Errorf("go %s failed in %d module(s): %s", command, len(failures), moduleList(failures))
}

func moduleList(items []result) string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.module)
	}
	return strings.Join(names, ", ")
}

// boundaryCheck 校验每个模块只 import 了清单允许的内部模块（含传递闭包）。
func boundaryCheck(root string, manifest *modgen.Manifest) error {
	byPath := manifest.ByPath()
	moduleByImport := make(map[string]string, len(manifest.Modules))
	for _, module := range manifest.Modules {
		moduleByImport[module.Module] = module.Path
	}

	var problems []string
	for _, module := range manifest.Modules {
		if module.Path == "." {
			continue
		}
		allowed := closure(module.Path, byPath)

		dir := filepath.Join(root, filepath.FromSlash(module.Path))
		cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", "./...")
		cmd.Dir = dir
		out, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("go list in %s: %w", module.Path, err)
		}
		seen := make(map[string]bool)
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || seen[line] {
				continue
			}
			seen[line] = true
			owner, ok := ownerModule(line, moduleByImport)
			if !ok || owner == module.Path {
				continue
			}
			if !allowed[owner] {
				problems = append(problems, fmt.Sprintf("%s imports %s (module %s not in deps)", module.Path, line, owner))
			}
		}
	}

	if len(problems) == 0 {
		fmt.Printf("dependency boundaries ok (%d modules)\n", len(manifest.Modules))
		return nil
	}
	sort.Strings(problems)
	for _, problem := range problems {
		fmt.Fprintln(os.Stderr, "  "+problem)
	}
	return fmt.Errorf("%d dependency boundary violations (see modules.json)", len(problems))
}

func closure(path string, byPath map[string]modgen.Module) map[string]bool {
	out := make(map[string]bool)
	var visit func(string)
	visit = func(current string) {
		module, ok := byPath[current]
		if !ok {
			return
		}
		for _, dep := range module.Deps {
			if out[dep] {
				continue
			}
			out[dep] = true
			visit(dep)
		}
	}
	visit(path)
	return out
}

// ownerModule 找到某个 import path 属于哪个内部模块（取最长匹配）。
func ownerModule(importPath string, moduleByImport map[string]string) (string, bool) {
	best := ""
	bestOwner := ""
	for modulePath, dir := range moduleByImport {
		if importPath == modulePath || strings.HasPrefix(importPath, modulePath+"/") {
			if len(modulePath) > len(best) {
				best = modulePath
				bestOwner = dir
			}
		}
	}
	if best == "" {
		return "", false
	}
	return bestOwner, true
}

func fail(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
