// schema_util.go 提供 GraphQL schema 构建过程中的通用辅助函数。
package graphql

import "strings"

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func firstNonEmpty(items ...string) string {
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			return item
		}
	}
	return ""
}
