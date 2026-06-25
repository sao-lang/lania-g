// matcher.go 实现 HTTP 路由的编译结果匹配逻辑。
package http

import (
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	httpprotocol "github.com/sao-lang/lania-g/protocol/http/v3/protocol"
)

type routeMatcher interface {
	Add(method, path string, handler *runtime.Handler)
}

type radixNode struct {
	static   map[string]*radixNode
	param    *radixNode
	paramKey string
	handler  *runtime.Handler
}

type radixMatcher struct {
	trees map[string]*radixNode
	all   *radixNode
}

// newRadixMatcher 创建一个最小 radix 风格匹配器。
// 它只承担运行时查路由，不负责声明冲突检测；冲突已在 Compile 阶段处理。
func newRadixMatcher() *radixMatcher {
	return &radixMatcher{
		trees: make(map[string]*radixNode),
		all:   &radixNode{static: make(map[string]*radixNode)},
	}
}

// Add 向匹配器注册一条已编译的 HTTP 路由。
func (r *radixMatcher) Add(method, path string, handler *runtime.Handler) {
	if method == httpprotocol.AllMethod {
		r.addTo(r.all, path, handler)
		return
	}
	root := r.trees[method]
	if root == nil {
		root = &radixNode{static: make(map[string]*radixNode)}
		r.trees[method] = root
	}
	r.addTo(root, path, handler)
}

func (r *radixMatcher) addTo(root *radixNode, path string, handler *runtime.Handler) {
	current := root
	for _, segment := range splitPath(path) {
		if strings.HasPrefix(segment, ":") {
			if current.param == nil {
				current.param = &radixNode{
					static:   make(map[string]*radixNode),
					paramKey: strings.TrimPrefix(segment, ":"),
				}
			}
			current = current.param
			continue
		}
		// 静态段与参数段分开存储，匹配时静态段优先，保证 `/users/me` 不会被 `/:id` 抢走。
		if current.static == nil {
			current.static = make(map[string]*radixNode)
		}
		next := current.static[segment]
		if next == nil {
			next = &radixNode{static: make(map[string]*radixNode)}
			current.static[segment] = next
		}
		current = next
	}
	current.handler = handler
}

// Match 根据请求上下文匹配出 handler，并返回 path params。
// 规则是：先匹配当前 method，再回退到 `ALL` 树。
func (r *radixMatcher) Match(ctx *runtime.HandlerContext) (*runtime.Handler, map[string]string) {
	if ctx == nil || ctx.Request == nil {
		return nil, nil
	}
	params := make(map[string]string)
	if root := r.trees[ctx.Request.Method]; root != nil {
		if h := matchNode(root, splitPath(ctx.Request.Path), 0, params); h != nil {
			return h, params
		}
	}
	params = make(map[string]string)
	if h := matchNode(r.all, splitPath(ctx.Request.Path), 0, params); h != nil {
		return h, params
	}
	return nil, nil
}

// matchNode 递归匹配单棵树。
// 每一层都先尝试 static，再尝试 param；param 失败时还会回滚本层写入的 path param。
func matchNode(node *radixNode, segments []string, idx int, params map[string]string) *runtime.Handler {
	if node == nil {
		return nil
	}
	if idx == len(segments) {
		return node.handler
	}
	seg := segments[idx]
	if next := node.static[seg]; next != nil {
		if h := matchNode(next, segments, idx+1, params); h != nil {
			return h
		}
	}
	if node.param != nil {
		if params != nil && node.param.paramKey != "" {
			params[node.param.paramKey] = seg
		}
		if h := matchNode(node.param, segments, idx+1, params); h != nil {
			return h
		}
		if params != nil && node.param.paramKey != "" {
			delete(params, node.param.paramKey)
		}
	}
	return nil
}

// splitPath 把 `/users/:id` 这类路径拆成稳定的 segment 列表，忽略首尾多余 `/`。
func splitPath(path string) []string {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return nil
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
