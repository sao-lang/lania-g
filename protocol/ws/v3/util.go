// util.go 提供 WS adapter 内部复用的通用辅助函数。
package ws

import "reflect"

// instancePointer 返回实例的指针地址（uintptr），用于 owner 消歧与索引。
//
// 规则：
// - 若 v 不是指针但可取地址，会先取 Addr()
// - 若最终不是指针类型，则返回 0
func instancePointer(v any) uintptr {
	if v == nil {
		return 0
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr && rv.CanAddr() {
		rv = rv.Addr()
	}
	if rv.Kind() != reflect.Ptr {
		return 0
	}
	return rv.Pointer()
}

// normalizeGatewayToken 将任意 gateway 实例的类型规范化为“指针类型 token”（*T）。
//
// 目的：统一 token 形态，避免 T 与 *T 在容器/owner 索引中不一致。
func normalizeGatewayToken(v any) reflect.Type {
	if v == nil {
		return nil
	}
	t := reflect.TypeOf(v)
	if t.Kind() != reflect.Ptr {
		t = reflect.PointerTo(t)
	}
	return t
}
