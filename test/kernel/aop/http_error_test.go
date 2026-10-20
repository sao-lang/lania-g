package aop_test

import . "github.com/sao-lang/lania-g/kernel/v3/aop"
import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestHttpExceptionConstructors(t *testing.T) {
	cases := []struct {
		name   string
		err    *HttpException
		status int
	}{
		{"paymentRequired", PaymentRequiredException(""), http.StatusPaymentRequired},
		{"methodNotAllowed", MethodNotAllowedException("", "GET", "POST"), http.StatusMethodNotAllowed},
		{"lengthRequired", LengthRequiredException(""), http.StatusLengthRequired},
		{"preconditionFailed", PreconditionFailedException(""), http.StatusPreconditionFailed},
		{"uriTooLong", URITooLongException(""), http.StatusRequestURITooLong},
		{"rangeNotSatisfiable", RangeNotSatisfiableException(""), http.StatusRequestedRangeNotSatisfiable},
		{"locked", LockedException(""), http.StatusLocked},
		{"failedDependency", FailedDependencyException(""), http.StatusFailedDependency},
		{"preconditionRequired", PreconditionRequiredException(""), http.StatusPreconditionRequired},
		{"tooManyRequests", TooManyRequestsException("", 0), http.StatusTooManyRequests},
		{"headerTooLarge", RequestHeaderFieldsTooLargeException(""), http.StatusRequestHeaderFieldsTooLarge},
		{"legal", UnavailableForLegalReasonsException(""), http.StatusUnavailableForLegalReasons},
		{"versionNotSupported", HTTPVersionNotSupportedException(""), http.StatusHTTPVersionNotSupported},
		{"insufficientStorage", InsufficientStorageException(""), http.StatusInsufficientStorage},
		{"loopDetected", LoopDetectedException(""), http.StatusLoopDetected},
		{"networkAuth", NetworkAuthenticationRequiredException(""), http.StatusNetworkAuthenticationRequired},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.Status != tc.status {
				t.Fatalf("status = %d, want %d", tc.err.Status, tc.status)
			}
			if tc.err.Message == "" {
				t.Fatalf("message should fall back to status text")
			}
		})
	}
}

func TestHttpExceptionHeadersAndUnwrap(t *testing.T) {
	cause := errors.New("underlying")
	err := MethodNotAllowedException("nope", "GET", "POST").
		WithCode("METHOD_NOT_ALLOWED").
		WithDetails(map[string]string{"field": "id"}).
		WithCause(cause)

	if got := err.Headers["Allow"]; got != "GET, POST" {
		t.Fatalf("Allow = %q", got)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is should see the cause")
	}
	if err.Code != "METHOD_NOT_ALLOWED" || err.Details == nil {
		t.Fatalf("code/details not set: %+v", err)
	}
}

func TestTooManyRequestsRetryAfter(t *testing.T) {
	err := TooManyRequestsException("slow down", 3*time.Second)
	if got := err.Headers["Retry-After"]; got != "3" {
		t.Fatalf("Retry-After = %q, want 3", got)
	}
}

func TestFromError(t *testing.T) {
	t.Run("http exception passes through", func(t *testing.T) {
		got, ok := FromError(ConflictException("dup"))
		if !ok || got.Status != http.StatusConflict {
			t.Fatalf("got=%v ok=%v", got, ok)
		}
	})

	t.Run("wrapped http exception", func(t *testing.T) {
		wrapped := errors.Join(errors.New("ctx"), ForbiddenException("no"))
		got, ok := FromError(wrapped)
		if !ok || got.Status != http.StatusForbidden {
			t.Fatalf("got=%v ok=%v", got, ok)
		}
	})

	t.Run("validation exception maps to 422", func(t *testing.T) {
		validation := NewValidationException("bad input",
			ValidationError{Field: "Email", Message: "invalid"})
		got, ok := FromError(validation)
		if !ok || got.Status != http.StatusUnprocessableEntity {
			t.Fatalf("got=%v ok=%v", got, ok)
		}
		if got.Details == nil {
			t.Fatalf("details should carry field errors")
		}
	})

	t.Run("nil", func(t *testing.T) {
		if _, ok := FromError(nil); ok {
			t.Fatalf("nil should not be convertible")
		}
	})

	t.Run("unknown error maps to 500", func(t *testing.T) {
		got, ok := FromError(errors.New("boom"))
		if !ok || got.Status != http.StatusInternalServerError {
			t.Fatalf("got=%v ok=%v", got, ok)
		}
	})
}

func TestStatusFromKind(t *testing.T) {
	if got := StatusFromKind("Validation"); got != http.StatusUnprocessableEntity {
		t.Fatalf("validation kind = %d", got)
	}
	if got := StatusFromKind("Unknown"); got != 0 {
		t.Fatalf("unknown kind = %d, want 0", got)
	}
}
