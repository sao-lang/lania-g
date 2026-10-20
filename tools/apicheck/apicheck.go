// Package apicheck 提取 Go 包的导出 API 快照，用于发现无意的破坏性变更。
//
// 它只依赖 golang.org/x/tools/go/packages 与 go/types，输出是稳定的纯文本：
//
//	<import path>
//	  func  Name(signature)
//	  type  Name <underlying>
//	  method Type.Method(signature)
//	  var   Name Type
//	  const Name Type
//
// 输出按字典序排序，因此可以直接作为 golden 文件做 diff。
package apicheck

import (
	"fmt"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Options 控制从哪些包、哪些目录提取快照。
type Options struct {
	// Dir 是仓库根目录，用于定位 go.work 与快照文件。
	Dir string
	// Patterns 传给 packages.Load，每个模块内默认为 "./..."。
	Patterns []string
	// Exclude 是需要跳过的 import path 前缀（cmd/test/website/tools 等）。
	Exclude []string
}

// DefaultOptions 返回仓库默认配置：跳过 demo、测试、文档站与工具自身。
func DefaultOptions(dir string) Options {
	return Options{
		Dir:      dir,
		Patterns: []string{"./..."},
		Exclude: []string{
			"github.com/sao-lang/lania-g/tools",
			"github.com/sao-lang/lania-g/test",
			"github.com/sao-lang/lania-g/cmd",
			"github.com/sao-lang/lania-g/website",
		},
	}
}

// Extract 返回 `import path -> 排序后的符号行`。
//
// go.work 的 `use` 指令决定要扫描哪些模块；每个模块单独加载，
// 因为 `./...` 在 workspace 根目录下不跨模块。
func Extract(opts Options) (map[string][]string, error) {
	if len(opts.Patterns) == 0 {
		opts.Patterns = []string{"./..."}
	}
	moduleDirs, err := WorkspaceModuleDirs(opts.Dir)
	if err != nil {
		return nil, err
	}
	if len(moduleDirs) == 0 {
		return nil, fmt.Errorf("no modules found in %s/go.work", opts.Dir)
	}

	out := make(map[string][]string)
	for _, moduleDir := range moduleDirs {
		cfg := &packages.Config{
			Mode: packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedModule,
			Dir:  moduleDir,
		}
		pkgs, loadErr := packages.Load(cfg, opts.Patterns...)
		if loadErr != nil {
			return nil, fmt.Errorf("load packages in %s: %w", moduleDir, loadErr)
		}
		for _, pkg := range pkgs {
			if pkg.Types == nil || pkg.TypesInfo == nil {
				continue
			}
			if excluded(pkg.PkgPath, opts.Exclude) {
				continue
			}
			if pkg.Name == "main" || strings.HasSuffix(pkg.PkgPath, ".test") {
				continue
			}
			symbols := describePackage(pkg.Types)
			if len(symbols) == 0 {
				continue
			}
			out[pkg.PkgPath] = symbols
		}
	}
	return out, nil
}

// WorkspaceModuleDirs 解析 go.work 的 use 指令，返回绝对路径列表。
func WorkspaceModuleDirs(root string) ([]string, error) {
	content, err := os.ReadFile(filepath.Join(root, "go.work"))
	if err != nil {
		return nil, fmt.Errorf("read go.work: %w", err)
	}
	var dirs []string
	inUse := false
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
		line = strings.Trim(line, "\"")
		if line == "" {
			continue
		}
		dirs = append(dirs, filepath.Join(root, filepath.FromSlash(line)))
	}
	sort.Strings(dirs)
	return dirs, nil
}

func excluded(importPath string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if prefix == "" {
			continue
		}
		if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
			return true
		}
	}
	return false
}

// describePackage 提取单个包的导出符号行。
func describePackage(pkg *types.Package) []string {
	scope := pkg.Scope()
	qualifier := types.RelativeTo(pkg)
	lines := make([]string, 0, scope.Len())
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if obj == nil || !obj.Exported() {
			continue
		}
		switch item := obj.(type) {
		case *types.Func:
			lines = append(lines, "func  "+item.Name()+strings.TrimPrefix(types.TypeString(item.Type(), qualifier), "func"))
		case *types.TypeName:
			lines = append(lines, "type  "+item.Name()+" "+underlyingString(item, qualifier))
			lines = append(lines, describeMethods(item.Type(), qualifier)...)
		case *types.Var:
			lines = append(lines, "var   "+item.Name()+" "+types.TypeString(item.Type(), qualifier))
		case *types.Const:
			lines = append(lines, "const "+item.Name()+" "+types.TypeString(item.Type(), qualifier))
		}
	}
	sort.Strings(lines)
	return lines
}

// underlyingString 返回类型定义右侧的内容。
//
// 具名类型取 underlying（这样 struct 的字段变化也会被捕捉到）；
// 类型别名保留原类型。
func underlyingString(item *types.TypeName, qualifier types.Qualifier) string {
	if item.IsAlias() {
		return "= " + types.TypeString(item.Type(), qualifier)
	}
	if named, ok := item.Type().(*types.Named); ok {
		return types.TypeString(named.Underlying(), qualifier)
	}
	return types.TypeString(item.Type(), qualifier)
}

// describeMethods 返回某个具名类型上导出的方法（含指针接收者）。
func describeMethods(typ types.Type, qualifier types.Qualifier) []string {
	named, ok := typ.(*types.Named)
	if !ok {
		return nil
	}
	lines := make([]string, 0, named.NumMethods())
	for i := 0; i < named.NumMethods(); i++ {
		method := named.Method(i)
		if method == nil || !method.Exported() {
			continue
		}
		signature := types.TypeString(method.Type(), qualifier)
		lines = append(lines, "method "+named.Obj().Name()+"."+method.Name()+strings.TrimPrefix(signature, "func"))
	}
	return lines
}

// Render 把快照渲染成稳定的文本（包按 import path 排序）。
func Render(snapshot map[string][]string) string {
	paths := make([]string, 0, len(snapshot))
	for path := range snapshot {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var builder strings.Builder
	builder.WriteString("# lania-g exported API snapshot\n")
	builder.WriteString("# 由 tools/apicheck 生成，请勿手工编辑。更新方式：go run ./tools/cmd/apicheck -update\n")
	for _, path := range paths {
		builder.WriteString("\n")
		builder.WriteString(path)
		builder.WriteString("\n")
		for _, line := range snapshot[path] {
			builder.WriteString("  ")
			builder.WriteString(line)
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

// FilePath 返回默认快照文件路径。
func FilePath(dir string) string {
	return filepath.Join(dir, "api", "api.txt")
}

// Write 把快照写入 path。
func Write(path string, snapshot map[string][]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(Render(snapshot)), 0o644)
}
