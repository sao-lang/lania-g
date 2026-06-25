package application

import (
	"testing"

	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
	httpprotocol "github.com/sao-lang/lania-g/protocol/http/v3/protocol"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

func mustTestAdapterAPI[T any](t *testing.T, app *Application, adapterID string) T {
	t.Helper()
	api, ok := app.API(adapterID)
	if !ok {
		t.Fatalf("adapter api not found: %s", adapterID)
	}
	typed, ok := api.(T)
	if !ok {
		t.Fatalf("adapter api %s has unexpected type %T", adapterID, api)
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
