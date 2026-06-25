// required.go 提供 HTTP 绑定中 required 约束的判断辅助。
package http

import (
	"reflect"
	"strings"
)

// fieldRequired 同时兼容两类历史写法：
// - `required:"true"`
// - `binding:"required,..."`
func fieldRequired(tag reflect.StructTag) bool {
	if isRequiredTag(tag.Get(TagRequired)) {
		return true
	}
	binding := strings.ToLower(strings.TrimSpace(tag.Get("binding")))
	if binding == "" {
		return false
	}
	for _, part := range strings.Split(binding, ",") {
		if strings.TrimSpace(part) == "required" {
			return true
		}
	}
	return false
}
