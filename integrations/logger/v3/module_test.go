package logger

import (
	"bytes"

)

type nopWriter struct{ bytes.Buffer }

func (w *nopWriter) Close() error { return nil }

type auditLogger struct{}

func (auditLogger) LoggerName() string { return "audit" }

