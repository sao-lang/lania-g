package enhancers_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	"github.com/sao-lang/lania-g/kernel/v3/runtime/enhancers"
)

func newExecContext() (*aop.ExecutionContext, *runtime.HandlerContext) {
	hc := runtime.NewHandlerContext("http")
	hc.RouteKey = "http:GET:/users/:id"
	hc.Request.Method = http.MethodGet
	hc.Request.Path = "/users/1"
	hc.Response.Status = http.StatusOK
	execCtx := aop.NewExecutionContext(context.Background(), hc, nil, nil)
	execCtx.Protocol = string(hc.Protocol)
	execCtx.RouteKey = hc.RouteKey
	execCtx.Request = hc.Request
	execCtx.Response = hc.Response
	return execCtx, hc
}

func TestRecoveryConvertsPanic(t *testing.T) {
	mw := enhancers.Recovery(nil)
	execCtx, _ := newExecContext()

	err := mw(execCtx, func() error { panic("kaboom") })
	if err == nil {
		t.Fatalf("expected error from panic")
	}
	httpErr, ok := aop.FromError(err)
	if !ok || httpErr.Status != http.StatusInternalServerError {
		t.Fatalf("status = %v", httpErr)
	}
}

func TestRequestIDGeneratesAndPropagates(t *testing.T) {
	mw := enhancers.RequestID("X-Request-Id", func() string { return "fixed-id" })
	execCtx, hc := newExecContext()

	if err := mw(execCtx, func() error { return nil }); err != nil {
		t.Fatalf("middleware: %v", err)
	}
	if got := hc.Response.Headers["X-Request-Id"]; got != "fixed-id" {
		t.Fatalf("response header = %q", got)
	}
	if got := hc.Request.Headers["X-Request-Id"]; got != "fixed-id" {
		t.Fatalf("request header = %q", got)
	}
	if got, _ := hc.Get(enhancers.RequestIDMetadataKey); got != "fixed-id" {
		t.Fatalf("metadata = %v", got)
	}
}

func TestRequestIDReusesIncomingHeader(t *testing.T) {
	mw := enhancers.RequestID("", func() string { return "generated" })
	execCtx, hc := newExecContext()
	hc.Request.Headers[enhancers.DefaultRequestIDHeader] = "incoming"

	if err := mw(execCtx, func() error { return nil }); err != nil {
		t.Fatalf("middleware: %v", err)
	}
	if got := hc.Response.Headers[enhancers.DefaultRequestIDHeader]; got != "incoming" {
		t.Fatalf("response header = %q, want incoming", got)
	}
}

func TestAccessLogRecordsEntry(t *testing.T) {
	var got enhancers.AccessEntry
	mw := enhancers.AccessLog(func(entry enhancers.AccessEntry) { got = entry })
	execCtx, hc := newExecContext()
	hc.Set(enhancers.RequestIDMetadataKey, "rid-1")

	if err := mw(execCtx, func() error { hc.Response.Status = http.StatusCreated; return nil }); err != nil {
		t.Fatalf("middleware: %v", err)
	}
	if got.Status != http.StatusCreated || got.Path != "/users/1" || got.Method != http.MethodGet {
		t.Fatalf("entry = %+v", got)
	}
	if got.RequestID != "rid-1" {
		t.Fatalf("request id = %q", got.RequestID)
	}
}

func TestAccessLogMapsErrorStatus(t *testing.T) {
	var got enhancers.AccessEntry
	mw := enhancers.AccessLog(func(entry enhancers.AccessEntry) { got = entry })
	execCtx, _ := newExecContext()

	_ = mw(execCtx, func() error { return aop.ConflictException("dup") })
	if got.Status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", got.Status)
	}
}

func TestTimeoutMapsDeadlineToGatewayTimeout(t *testing.T) {
	mw := enhancers.Timeout(5 * time.Millisecond)
	execCtx, hc := newExecContext()

	err := mw(execCtx, func() error {
		<-hc.GetContext().Done()
		return errors.New("downstream failed")
	})
	httpErr, ok := aop.FromError(err)
	if !ok || httpErr.Status != http.StatusGatewayTimeout {
		t.Fatalf("status = %v err=%v", httpErr, err)
	}
}

func TestErrorEnvelopeWritesResponse(t *testing.T) {
	filter := enhancers.ErrorEnvelope(nil)
	execCtx, hc := newExecContext()

	err := filter(aop.MethodNotAllowedException("nope", "GET"), execCtx)
	if err != nil {
		t.Fatalf("filter should consume the error, got %v", err)
	}
	if hc.Response.Status != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", hc.Response.Status)
	}
	if got := hc.Response.Headers["Allow"]; got != "GET" {
		t.Fatalf("Allow = %q", got)
	}
	body, ok := hc.Response.Body.(enhancers.ErrorBody)
	if !ok || body.Code != "HTTP-405" {
		t.Fatalf("body = %#v", hc.Response.Body)
	}
}

func TestErrorEnvelopeIgnoresNonError(t *testing.T) {
	filter := enhancers.ErrorEnvelope(nil)
	execCtx, _ := newExecContext()
	if err := filter("not-an-error", execCtx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
