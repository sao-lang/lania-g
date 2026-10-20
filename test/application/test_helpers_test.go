package application_test

// 测试位于仓库根目录 test/，通过 dot-import 访问被测包。
import . "github.com/sao-lang/lania-g/application/v3"
import (
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
	httpprotocol "github.com/sao-lang/lania-g/protocol/http/v3/protocol"
)

func mustTestAdapter[T any](t *testing.T, app *Application, adapterID string) T {
	t.Helper()
	adp, ok := app.Adapter(adapterID)
	if !ok {
		t.Fatalf("adapter not found: %s", adapterID)
	}
	typed, ok := adp.(T)
	if !ok {
		t.Fatalf("adapter %s has unexpected type %T", adapterID, adp)
	}
	return typed
}

func executeTestHTTPPing(t *testing.T, app *Application, path string) string {
	t.Helper()
	ctx := runtime.NewHandlerContext(httpprotocol.Protocol)
	ctx.Request.Method = "GET"
	ctx.Request.Path = path
	out, err := app.Runtime().Execute(ctx)
	if err != nil {
		t.Fatalf("execute http route: %v", err)
	}
	value, ok := out.(string)
	if !ok {
		t.Fatalf("output type = %T, want string", out)
	}
	return value
}

var _ = httpadapter.AdapterID
