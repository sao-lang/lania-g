package module_test

// 测试位于仓库根目录 test/，通过 dot-import 访问被测包。
import . "github.com/sao-lang/lania-g/kernel/v3/module"
import (
	"testing"
	"time"
)

// 这个用例验证：带 root 初始化的 NewModuleLoader 不会因为锁顺序问题发生死锁。
func TestNewModuleLoader_WithRootDoesNotDeadlock(t *testing.T) {
	root := New(Metadata{})
	done := make(chan *ModuleLoader, 1)

	go func() {
		done <- NewModuleLoader(root)
	}()

	select {
	case loader := <-done:
		if loader == nil {
			t.Fatalf("expected loader")
		}
		if loader.GetRootModule() == nil {
			t.Fatalf("expected root module")
		}
		if loader.GetModuleRef() == nil {
			t.Fatalf("expected module ref")
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("NewModuleLoader(root) timed out")
	}
}
