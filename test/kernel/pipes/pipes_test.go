package pipes_test

import (
	"strings"
	"testing"
	"time"

	"github.com/sao-lang/lania-g/kernel/v3/aop/pipes"
)

func TestTrimSpacePipe(t *testing.T) {
	pipe := pipes.NewTrimSpacePipe()

	got, err := pipe.Transform("  hi  ", nil)
	if err != nil || got != "hi" {
		t.Fatalf("string = %v err=%v", got, err)
	}

	got, err = pipe.Transform([]string{" a ", "b "}, nil)
	if err != nil {
		t.Fatalf("slice err=%v", err)
	}
	values := got.([]string)
	if values[0] != "a" || values[1] != "b" {
		t.Fatalf("slice = %v", values)
	}

	type form struct {
		Name  string
		Email string
		Age   int
	}
	got, err = pipe.Transform(form{Name: " alice ", Email: " a@b.c ", Age: 3}, nil)
	if err != nil {
		t.Fatalf("struct err=%v", err)
	}
	trimmed := got.(form)
	if trimmed.Name != "alice" || trimmed.Email != "a@b.c" || trimmed.Age != 3 {
		t.Fatalf("struct = %+v", trimmed)
	}
}

func TestParseTimePipe(t *testing.T) {
	pipe := pipes.NewParseTimePipe("")

	got, err := pipe.Transform("2026-10-20", nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	parsed := got.(time.Time)
	if parsed.Year() != 2026 || parsed.Month() != time.October || parsed.Day() != 20 {
		t.Fatalf("parsed = %v", parsed)
	}

	if _, err := pipe.Transform("not-a-time", nil); err == nil {
		t.Fatalf("expected parse error")
	}
	if _, err := pipe.Transform(42, nil); err == nil {
		t.Fatalf("expected type error")
	}
}

func TestParseEnumPipe(t *testing.T) {
	pipe := pipes.NewParseEnumPipe("admin", "member")

	if got, err := pipe.Transform("ADMIN", nil); err != nil || got != "ADMIN" {
		t.Fatalf("got=%v err=%v", got, err)
	}
	if _, err := pipe.Transform("guest", nil); err == nil {
		t.Fatalf("expected enum error")
	} else if !strings.Contains(err.Error(), "admin") {
		t.Fatalf("error should list allowed values: %v", err)
	}
}

func TestRequiredPipe(t *testing.T) {
	pipe := pipes.NewRequiredPipe()

	for _, empty := range []any{nil, "", []string{}, map[string]int{}} {
		if _, err := pipe.Transform(empty, nil); err == nil {
			t.Fatalf("expected required error for %#v", empty)
		}
	}
	if got, err := pipe.Transform("ok", nil); err != nil || got != "ok" {
		t.Fatalf("got=%v err=%v", got, err)
	}
}
