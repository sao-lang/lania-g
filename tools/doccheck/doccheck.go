// Package doccheck 校验文档里的 Go 代码块：
//
//  1. 语法必须正确（go/parser）
//  2. 形如 `di.ProviderFromValue(...)` 的框架符号必须真实存在于对应包
//
// 第 2 点是本工具的核心：它拦住"文档里写了一个不存在的 API"这类
// 最难靠人工发现、也最伤使用者信任的问题。
package doccheck

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Finding 是一条文档问题。
type Finding struct {
	File    string
	Line    int
	Message string
}

// String 以 `file:line: message` 形式输出。
func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s", f.File, f.Line, f.Message)
}

// Options 控制扫描范围。
type Options struct {
	// Root 是仓库根目录。
	Root string
	// DocsDir 是文档目录（相对 Root）。
	DocsDir string
	// Snapshot 是 api/api.txt（相对 Root）。
	Snapshot string
}

// DefaultOptions 返回仓库默认配置。
func DefaultOptions(root string) Options {
	return Options{
		Root:     root,
		DocsDir:  "website/content/docs",
		Snapshot: "api/api.txt",
	}
}

// SymbolIndex 是 `import path -> 导出符号集合` 的索引。
type SymbolIndex map[string]map[string]bool

// LoadIndex 从 api 快照构建索引。
//
// 快照格式（见 tools/apicheck）：
//
//	github.com/sao-lang/lania-g/kernel/v3/aop
//	  func  GuardFunc(...)
//	  type  HttpException ...
func LoadIndex(path string) (SymbolIndex, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}
	index := make(SymbolIndex)
	var current string
	for _, raw := range strings.Split(string(content), "\n") {
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		if !strings.HasPrefix(raw, " ") {
			current = raw
			index[current] = make(map[string]bool)
			continue
		}
		if current == "" {
			continue
		}
		if name := symbolName(strings.TrimSpace(raw)); name != "" {
			index[current][name] = true
		}
	}
	return index, nil
}

// symbolName 从快照行里取出符号名。
func symbolName(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return ""
	}
	name := fields[1]
	switch fields[0] {
	case "func", "type", "var", "const":
		// 泛型函数/类型的名字后面可能紧跟 `[T any]` 或 `(...)`。
		if idx := strings.IndexAny(name, "[(="); idx >= 0 {
			name = name[:idx]
		}
		return strings.TrimSpace(name)
	case "method":
		if idx := strings.IndexAny(name, "[(="); idx >= 0 {
			name = name[:idx]
		}
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			return name[idx+1:]
		}
		return name
	default:
		return ""
	}
}

// aliasPattern 匹配文档里的 `alias.Symbol` 调用/引用。
var aliasPattern = regexp.MustCompile(`\b([a-z][a-zA-Z0-9_]{1,20})\.([A-Z][A-Za-z0-9_]*)`)

// packageAliases 把文档里常用的包别名映射到 import path 后缀。
//
// 只收录框架自己的包：外部库的符号无法从快照里验证，忽略。
var packageAliases = map[string]string{
	"aop":          "/kernel/v3/aop",
	"pipes":        "/kernel/v3/aop/pipes",
	"di":           "/kernel/v3/di",
	"module":       "/kernel/v3/module",
	"registry":     "/kernel/v3/registry",
	"compiler":     "/kernel/v3/compiler",
	"runtime":      "/kernel/v3/runtime",
	"enhancers":    "/kernel/v3/runtime/enhancers",
	"application":  "/application/v3",
	"httpadapter":  "/protocol/http/v3",
	"grpcadapter":  "/protocol/grpc/v3",
	"graphqladapter": "/protocol/graphql/v3",
	"wsadapter":    "/protocol/ws/v3",
	"mqadapter":    "/protocol/mq/v3",
	"httpprotocol": "/protocol/http/v3/protocol",
	"httpbinding":  "/protocol/http/v3/binding",
	"graphql":      "/protocol/graphql/v3",
	"ws":           "/protocol/ws/v3",
	"mq":           "/protocol/mq/v3",
	"scheduler":    "/protocol/scheduler/v3",
	"swagger":      "/integrations/swagger/v3",
	"terminus":     "/integrations/terminus/v3",
	"otel":         "/integrations/otel/v3",
	"resilience":   "/integrations/resilience/v3",
}

// Run 扫描文档目录，返回全部问题。
func Run(opts Options) ([]Finding, error) {
	if opts.DocsDir == "" || opts.Snapshot == "" {
		return nil, fmt.Errorf("DocsDir and Snapshot are required")
	}
	index, err := LoadIndex(filepath.Join(opts.Root, filepath.FromSlash(opts.Snapshot)))
	if err != nil {
		return nil, err
	}

	var findings []Finding
	docsDir := filepath.Join(opts.Root, filepath.FromSlash(opts.DocsDir))
	walkErr := filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, relErr := filepath.Rel(opts.Root, path)
		if relErr != nil {
			rel = path
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		findings = append(findings, checkFile(filepath.ToSlash(rel), string(content), index)...)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, nil
}

// codeBlock 是文档里的一段 Go 代码。
type codeBlock struct {
	StartLine int
	Content   string
	// CheckSyntax 表示该块显式标记了 `check`（```go check）。
	// 文档里的示例常包含省略号、裸签名等"非完整源码"，因此语法检查必须显式开启。
	CheckSyntax bool
}

// extractGoBlocks 提取所有 ```go 代码块。
func extractGoBlocks(markdown string) []codeBlock {
	var blocks []codeBlock
	lines := strings.Split(markdown, "\n")
	inBlock := false
	start := 0
	check := false
	var buffer []string

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inBlock {
				blocks = append(blocks, codeBlock{
					StartLine:   start,
					Content:     strings.Join(buffer, "\n"),
					CheckSyntax: check,
				})
				inBlock = false
				buffer = nil
				continue
			}
			info := strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			if info == "go" || strings.HasPrefix(info, "go ") {
				inBlock = true
				start = i + 2
				check = strings.Contains(info, "check")
				buffer = nil
			}
			continue
		}
		if inBlock {
			buffer = append(buffer, line)
		}
	}
	return blocks
}

func checkFile(relPath, content string, index SymbolIndex) []Finding {
	var findings []Finding
	for _, block := range extractGoBlocks(content) {
		if strings.TrimSpace(block.Content) == "" {
			continue
		}
		if block.CheckSyntax && !parses(block.Content) {
			findings = append(findings, Finding{
				File:    relPath,
				Line:    block.StartLine,
				Message: "go 代码块语法错误: " + parseProblem(block.Content),
			})
			continue
		}

		for _, match := range aliasPattern.FindAllStringSubmatch(block.Content, -1) {
			alias, symbol := match[1], match[2]
			if shadowed(block.Content, alias) {
				// 文档在该块里用同名别名导入了外部库（如 google.golang.org/grpc），
				// 此时无法用框架快照判定，跳过。
				continue
			}
			suffix, known := packageAliases[alias]
			if !known {
				continue
			}
			pkgPath, found := findPackage(index, suffix)
			if !found {
				continue
			}
			if index[pkgPath][symbol] {
				continue
			}
			findings = append(findings, Finding{
				File:    relPath,
				Line:    block.StartLine,
				Message: fmt.Sprintf("%s.%s 不存在于 %s", alias, symbol, pkgPath),
			})
		}
	}
	return findings
}

// parses 尝试把代码块解析成合法的 Go 源码。
//
// 文档片段有两种形态：顶层声明（type/func/const）与语句列表，
// 两种都试一次，任一通过即可。
func parses(content string) bool {
	return parseProblem(content) == ""
}

func parseProblem(content string) string {
	trimmed := strings.TrimSpace(content)
	candidates := []string{
		"package snippet\n\n" + content + "\n",
		"package snippet\n\nfunc _snippet() {\n" + content + "\n}\n",
	}
	if strings.HasPrefix(trimmed, "package ") {
		candidates = candidates[:1]
	}
	var lastErr string
	for _, source := range candidates {
		fset := token.NewFileSet()
		if _, err := parser.ParseFile(fset, "snippet.go", source, 0); err == nil {
			return ""
		} else {
			lastErr = trimParseError(err)
		}
	}
	return lastErr
}

// shadowed 判断代码块里是否用该别名导入了框架之外的包。
func shadowed(content, alias string) bool {
	markers := map[string][]string{
		"grpc":   {`"google.golang.org/grpc"`},
		"http":   {`"net/http"`},
		"errors": {`"errors"`},
		"kerrors": {`"errors"`},
		"binding": {`"github.com/sao-lang/lania-g/protocol/grpc/v3/binding"`, `"github.com/sao-lang/lania-g/protocol/ws/v3/binding"`},
	}
	patterns, ok := markers[alias]
	if !ok {
		return false
	}
	for _, pattern := range patterns {
		if strings.Contains(content, pattern) {
			return true
		}
	}
	return false
}

// findPackage 用路径后缀匹配 import path（避免文档里写完整路径）。
func findPackage(index SymbolIndex, suffix string) (string, bool) {
	for path := range index {
		if strings.HasSuffix(path, suffix) {
			return path, true
		}
	}
	return "", false
}

func trimParseError(err error) string {
	message := err.Error()
	if idx := strings.Index(message, ":"); idx >= 0 {
		message = message[idx+1:]
	}
	return strings.TrimSpace(message)
}
