package runtime

import "testing"

// TestHandlerContext_Require 验证 HandlerContext.Require 的行为：
// - key 存在时返回 value 且 err=nil
// - key 不存在时返回 err
func TestHandlerContext_Require(t *testing.T) {
	ctx := NewHandlerContext("test")
	ctx.Set("trace_id", "abc")

	value, err := ctx.Require("trace_id")
	if err != nil {
		t.Fatalf("require existing key: %v", err)
	}
	if value != "abc" {
		t.Fatalf("value = %v, want abc", value)
	}

	if _, err := ctx.Require("missing"); err == nil {
		t.Fatalf("require missing key: want error")
	}
}
