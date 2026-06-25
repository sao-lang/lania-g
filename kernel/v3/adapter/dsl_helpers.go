// dsl_helpers.go 提供各协议 DSL 共享的轻量辅助函数。
package adapter

import (
	"maps"
	"reflect"
	goruntime "runtime"
	"strings"
)

// FindMethodName 从 receiver + handler 值里反推出 Go 方法名。
// 这是各协议 DSL 的关键辅助：DSL 接受 bound method / method expression，
// 但编译期最终只需要一个稳定的字符串方法名。
func FindMethodName(receiver any, handler any) string {
	receiverVal := reflect.ValueOf(receiver)
	receiverType := receiverVal.Type()
	handlerVal := reflect.ValueOf(handler)
	if !handlerVal.IsValid() || handlerVal.Kind() != reflect.Func {
		return ""
	}
	for i := 0; i < receiverType.NumMethod(); i++ {
		if receiverVal.Method(i).Pointer() == handlerVal.Pointer() {
			return receiverType.Method(i).Name
		}
	}
	// 第一轮按方法指针没命中时，再退回到 runtime 函数名。
	// 这是为了兼容 method value 包装、`-fm` 后缀等情况。
	if fn := goruntime.FuncForPC(handlerVal.Pointer()); fn != nil {
		name := fn.Name()
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			name = name[idx+1:]
		}
		name = strings.TrimSuffix(name, "-fm")
		for i := 0; i < receiverType.NumMethod(); i++ {
			if receiverType.Method(i).Name == name {
				return name
			}
		}
	}
	return ""
}

// MergeParamPipes 合并参数级 pipe 配置，并保证不原地修改输入 map。
// 执行顺序保持“base 在前、extra 在后”，这样路由级配置会自然追加到上层共享配置后面。
func MergeParamPipes(base map[int][]any, extra map[int][]any) map[int][]any {
	merged := make(map[int][]any, len(base)+len(extra))
	for idx, pipes := range base {
		merged[idx] = append([]any{}, pipes...)
	}
	for idx, pipes := range extra {
		merged[idx] = append(merged[idx], pipes...)
	}
	return merged
}

// CopyIntStringMap 返回 `map[int]string` 的浅拷贝。
// 主要用于 DSL/编译阶段做声明快照，避免多个 builder 共享同一底层 map。
func CopyIntStringMap(src map[int]string) map[int]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[int]string, len(src))
	maps.Copy(out, src)
	return out
}
