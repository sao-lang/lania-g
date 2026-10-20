package aop_test

import . "github.com/sao-lang/lania-g/kernel/v3/aop"
import (
	"errors"
	"testing"
)

func noopMiddleware(ctx *ExecutionContext, next func() error) error { return next() }

func TestScopedMiddlewareShouldApply(t *testing.T) {
	cases := []struct {
		name    string
		include []string
		exclude []string
		method  string
		path    string
		want    bool
	}{
		{name: "prefix wildcard matches root", include: []string{"/users/*"}, method: "GET", path: "/users", want: true},
		{name: "prefix wildcard matches child", include: []string{"/users/*"}, method: "GET", path: "/users/42", want: true},
		{name: "prefix wildcard is segment aware", include: []string{"/users/*"}, method: "GET", path: "/usersettings", want: false},
		{name: "path param matches one segment", include: []string{"/users/:id"}, method: "GET", path: "/users/42", want: true},
		{name: "path param rejects deeper path", include: []string{"/users/:id"}, method: "GET", path: "/users/42/posts", want: false},
		{name: "brace param", include: []string{"/users/{id}"}, method: "GET", path: "/users/42", want: true},
		{name: "match all", include: []string{"*"}, method: "GET", path: "/anything", want: true},
		{name: "empty include applies everywhere", method: "GET", path: "/anything", want: true},
		{name: "exact path", include: []string{"/users"}, method: "GET", path: "/users", want: true},
		{name: "exact path rejects child", include: []string{"/users"}, method: "GET", path: "/users/42", want: false},
		{name: "method rule matches", include: []string{"POST /users/*"}, method: "POST", path: "/users/1", want: true},
		{name: "method rule rejects other method", include: []string{"POST /users/*"}, method: "GET", path: "/users/1", want: false},
		{name: "ALL matches any method", include: []string{"ALL /users"}, method: "DELETE", path: "/users", want: true},
		{name: "method rule requires protocol method", include: []string{"GET /users"}, method: "", path: "/users", want: false},
		{name: "method rule matches ALL route", include: []string{"GET /users"}, method: "ALL", path: "/users", want: true},
		{name: "exclude wins over include", include: []string{"/users/*"}, exclude: []string{"DELETE /users/:id"}, method: "DELETE", path: "/users/1", want: false},
		{name: "exclude ignores other method", include: []string{"/users/*"}, exclude: []string{"DELETE /users/:id"}, method: "GET", path: "/users/1", want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scoped := ScopedMiddleware{
				Middlewares:   []MiddlewareFunc{noopMiddleware},
				IncludeRoutes: tc.include,
				ExcludeRoutes: tc.exclude,
			}
			if got := scoped.ShouldApply(tc.method, tc.path); got != tc.want {
				t.Fatalf("ShouldApply(%q, %q) = %v, want %v", tc.method, tc.path, got, tc.want)
			}
		})
	}
}

func TestMiddlewareConsumerCollectsIndependentBindings(t *testing.T) {
	consumer := NewMiddlewareConsumer()

	users := MiddlewareFunc(func(ctx *ExecutionContext, next func() error) error { return next() })
	admin := MiddlewareFunc(func(ctx *ExecutionContext, next func() error) error { return next() })

	consumer.Apply(users).ForRoutes("/users/*").Exclude("GET /users")
	consumer.Apply(admin).ForRoutes("GET /admin/*")

	if err := consumer.Err(); err != nil {
		t.Fatalf("unexpected consumer error: %v", err)
	}

	bindings := consumer.Bindings()
	if len(bindings) != 2 {
		t.Fatalf("bindings = %d, want 2", len(bindings))
	}
	if len(bindings[0].Middlewares) != 1 || len(bindings[1].Middlewares) != 1 {
		t.Fatalf("each binding should own exactly one middleware")
	}
	if !bindings[0].ShouldApply("GET", "/users/1") {
		t.Fatalf("users binding should apply to /users/1")
	}
	if bindings[0].ShouldApply("GET", "/users") {
		t.Fatalf("users binding should be excluded on GET /users")
	}
	if bindings[1].ShouldApply("GET", "/users/1") {
		t.Fatalf("admin binding should not apply to /users/1")
	}
	if !bindings[1].ShouldApply("GET", "/admin/stats") {
		t.Fatalf("admin binding should apply to GET /admin/stats")
	}
}

func TestMiddlewareConsumerRecordsInvalidType(t *testing.T) {
	consumer := NewMiddlewareConsumer()
	consumer.Apply(func() {}).ForRoutes("*")

	err := consumer.Err()
	if err == nil {
		t.Fatalf("expected error for unsupported middleware type")
	}
	var unsupported *ErrUnsupported
	if !errors.As(err, &unsupported) {
		t.Fatalf("error = %T, want *ErrUnsupported", err)
	}
	if unsupported.Kind != "middleware" {
		t.Fatalf("kind = %q, want middleware", unsupported.Kind)
	}
	if len(consumer.Bindings()) != 0 {
		t.Fatalf("invalid binding should not be recorded")
	}
}

func TestMiddlewareConsumerRequiresMiddleware(t *testing.T) {
	consumer := NewMiddlewareConsumer()
	consumer.Apply()
	if consumer.Err() == nil {
		t.Fatalf("expected error for empty Apply")
	}
}

func TestCoerceGuardsRejectsUnsupportedType(t *testing.T) {
	if _, err := CoerceGuards([]any{"not-a-guard"}); err == nil {
		t.Fatalf("expected error for unsupported guard type")
	}
}
