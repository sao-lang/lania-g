// Package modgen 以 modules.json 为唯一事实来源，维护多模块仓库的 go.mod / go.work。
//
// 为什么需要它：仓库有近 30 个 module，跨模块重构时要手工改 N 个 go.mod，
// 很容易漏掉 replace 或写错版本。这里把"模块清单"变成可校验、可生成的单一来源。
//
// 命令：
//
//	go run ./tools/cmd/modgen -check   # 校验（CI 用）
//	go run ./tools/cmd/modgen -write   # 按清单修正内部 require/replace
//	go run ./tools/cmd/modrun build    # 在所有模块里跑 go build
package modgen

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Module 是清单里的一项。
type Module struct {
	// Path 是相对仓库根的目录。
	Path string `json:"path"`
	// Module 是 module path。
	Module string `json:"module"`
	// Deps 是允许依赖的内部模块目录列表。
	Deps []string `json:"deps"`
}

// Manifest 是 modules.json 的结构。
type Manifest struct {
	Modules []Module `json:"modules"`
}

// Load 读取 root/modules.json。
func Load(root string) (*Manifest, error) {
	content, err := os.ReadFile(filepath.Join(root, "modules.json"))
	if err != nil {
		return nil, fmt.Errorf("read modules.json: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return nil, fmt.Errorf("parse modules.json: %w", err)
	}
	if len(manifest.Modules) == 0 {
		return nil, fmt.Errorf("modules.json has no modules")
	}
	seen := make(map[string]bool, len(manifest.Modules))
	for _, module := range manifest.Modules {
		if module.Path == "" || module.Module == "" {
			return nil, fmt.Errorf("module entry missing path/module: %+v", module)
		}
		if seen[module.Path] {
			return nil, fmt.Errorf("duplicate module path %q", module.Path)
		}
		seen[module.Path] = true
	}
	for _, module := range manifest.Modules {
		for _, dep := range module.Deps {
			if !seen[dep] {
				return nil, fmt.Errorf("module %q depends on unknown module %q", module.Path, dep)
			}
		}
	}
	return &manifest, nil
}

// ByPath 返回 path -> Module。
func (m *Manifest) ByPath() map[string]Module {
	out := make(map[string]Module, len(m.Modules))
	for _, module := range m.Modules {
		out[module.Path] = module
	}
	return out
}

// Version 返回内部依赖应使用的版本号。
//
// 规则：`/v3` 结尾的 module 用 v3.0.0，其余（tools/test 等）用 v0.0.0。
func Version(modulePath string) string {
	if strings.HasSuffix(modulePath, "/v3") {
		return "v3.0.0"
	}
	return "v0.0.0"
}

// Issue 是一条清单与仓库不一致的记录。
type Issue struct {
	Module  string
	Message string
}

// String 以单行文本描述一条不一致。
func (i Issue) String() string {
	return fmt.Sprintf("%-28s %s", i.Module, i.Message)
}

// 语义约定：
//
//   - `deps` 是**允许**依赖的内部模块（策略），可以比实际依赖更宽
//   - go.mod 里的实际 require 由 `go mod tidy` 计算得出
//   - modgen 只负责：require ⊆ deps、每个 require 都有正确的 replace、go.work 覆盖全部模块
//
// 这样既不会因为"策略里有但代码没用到"而报错，也能拦住越界依赖。
//
// Check 校验 go.mod 的内部 require/replace 与 go.work 是否与清单一致。
func Check(root string, manifest *Manifest) ([]Issue, error) {
	byPath := manifest.ByPath()
	var issues []Issue

	for _, module := range manifest.Modules {
		dir := filepath.Join(root, filepath.FromSlash(module.Path))
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
			issues = append(issues, Issue{module.Path, "go.mod missing"})
			continue
		}

		info, err := readGoMod(dir)
		if err != nil {
			issues = append(issues, Issue{module.Path, err.Error()})
			continue
		}
		if info.Module.Path != module.Module {
			issues = append(issues, Issue{module.Path, fmt.Sprintf("module line = %q, want %q", info.Module.Path, module.Module)})
		}

		allowed := make(map[string]bool, len(module.Deps))
		for _, depPath := range module.Deps {
			allowed[byPath[depPath].Module] = true
		}

		required := make(map[string]string)
		for _, req := range info.Require {
			if strings.HasPrefix(req.Path, "github.com/sao-lang/lania-g/") {
				required[req.Path] = req.Version
			}
		}
		for path, version := range required {
			if !allowed[path] {
				issues = append(issues, Issue{module.Path, "require not allowed by modules.json deps: " + path})
			}
			if want := Version(path); version != want {
				issues = append(issues, Issue{module.Path, fmt.Sprintf("require %s = %s, want %s", path, version, want)})
			}
		}

		gotReplaces := make(map[string]string)
		for _, rep := range info.Replace {
			if strings.HasPrefix(rep.Old.Path, "github.com/sao-lang/lania-g/") {
				gotReplaces[rep.Old.Path] = rep.New.Path
			}
		}
		for path := range required {
			gotTarget, ok := gotReplaces[path]
			if !ok {
				issues = append(issues, Issue{module.Path, "missing replace " + path})
				continue
			}
			want := relativeReplaceTarget(module.Path, path, byPath)
			if filepath.ToSlash(filepath.Clean(gotTarget)) != want {
				issues = append(issues, Issue{module.Path, fmt.Sprintf("replace %s -> %s (want %s)", path, gotTarget, want)})
			}
		}
		for path := range gotReplaces {
			if _, ok := required[path]; !ok {
				issues = append(issues, Issue{module.Path, "stale replace (no matching require): " + path})
			}
		}
	}

	workIssues, err := checkGoWork(root, manifest)
	if err != nil {
		return issues, err
	}
	issues = append(issues, workIssues...)

	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Module != issues[j].Module {
			return issues[i].Module < issues[j].Module
		}
		return issues[i].Message < issues[j].Message
	})
	return issues, nil
}
// checkGoWork 校验 go.work 的 use 列表与清单一致（根模块不参与 workspace）。
func checkGoWork(root string, manifest *Manifest) ([]Issue, error) {
	content, err := os.ReadFile(filepath.Join(root, "go.work"))
	if err != nil {
		return nil, fmt.Errorf("read go.work: %w", err)
	}
	inUse := false
	got := make(map[string]bool)
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "" || strings.HasPrefix(line, "//"):
			continue
		case strings.HasPrefix(line, "use ("):
			inUse = true
			continue
		case inUse && line == ")":
			inUse = false
			continue
		case strings.HasPrefix(line, "use "):
			line = strings.TrimSpace(strings.TrimPrefix(line, "use "))
		case !inUse:
			continue
		}
		got[strings.Trim(strings.TrimSpace(line), "\"")] = true
	}

	want := make(map[string]bool)
	for _, module := range manifest.Modules {
		if module.Path == "." {
			continue
		}
		want["./"+filepath.ToSlash(module.Path)] = true
	}

	var issues []Issue
	for path := range want {
		if !got[path] {
			issues = append(issues, Issue{"go.work", "missing use " + path})
		}
	}
	for path := range got {
		if !want[path] {
			issues = append(issues, Issue{"go.work", "unexpected use " + path})
		}
	}
	return issues, nil
}

// Write 按清单修正每个 go.mod 的 replace（并统一内部 require 版本）。
//
// 它不会"凭清单添加 require"——require 是否存在由 `go mod tidy` 决定；
// 这里只保证"有 require 就得有正确的 replace"，以及版本号统一。
func Write(root string, manifest *Manifest) error {
	byPath := manifest.ByPath()

	for _, module := range manifest.Modules {
		dir := filepath.Join(root, filepath.FromSlash(module.Path))
		info, err := readGoMod(dir)
		if err != nil {
			return fmt.Errorf("%s: %w", module.Path, err)
		}

		required := make(map[string]bool)
		for _, req := range info.Require {
			if !strings.HasPrefix(req.Path, "github.com/sao-lang/lania-g/") {
				continue
			}
			required[req.Path] = true
			if req.Version != Version(req.Path) {
				if err := goModEdit(dir, "-require="+req.Path+"@"+Version(req.Path)); err != nil {
					return err
				}
			}
		}

		// replace 与 require 一一对应：先清空再按 require 重建。
		for _, rep := range info.Replace {
			if strings.HasPrefix(rep.Old.Path, "github.com/sao-lang/lania-g/") {
				if err := goModEdit(dir, "-dropreplace="+rep.Old.Path); err != nil {
					return err
				}
			}
		}
		for path := range required {
			target := relativeReplaceTarget(module.Path, path, byPath)
			if err := goModEdit(dir, "-replace="+path+"="+target); err != nil {
				return err
			}
		}
	}
	return nil
}
// relativeReplaceTarget 计算 replace 指向的相对路径（相对当前模块目录）。
func relativeReplaceTarget(fromPath, depModule string, byPath map[string]Module) string {
	for path, module := range byPath {
		if module.Module != depModule {
			continue
		}
		rel, err := filepath.Rel(filepath.FromSlash(fromPath), filepath.FromSlash(path))
		if err != nil {
			return path
		}
		return filepath.ToSlash(rel)
	}
	return depModule
}

type goModInfo struct {
	Module  struct{ Path string }
	Require []struct {
		Path    string
		Version string
	}
	Replace []struct {
		Old struct{ Path, Version string }
		New struct{ Path, Version string }
	}
}

func readGoMod(dir string) (*goModInfo, error) {
	cmd := exec.Command("go", "mod", "edit", "-json")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go mod edit -json: %w", err)
	}
	var info goModInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("parse go.mod: %w", err)
	}
	return &info, nil
}

func goModEdit(dir string, args ...string) error {
	cmd := exec.Command("go", append([]string{"mod", "edit"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod edit %v in %s: %w: %s", args, dir, err, strings.TrimSpace(string(out)))
	}
	return nil
}
