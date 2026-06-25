// types.go 定义 logger 集成对外暴露的公共类型、选项与包装结构。
package logger

import (
	"context"
	"io"
	"time"
)

// Logger 定义框架统一使用的日志接口。
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	Fatal(msg string, fields ...Field)

	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})

	With(fields ...Field) Logger
	WithContext(ctx context.Context) Logger

	Sync() error
}

// Hook 定义日志写出前的扩展钩子。
type Hook interface {
	Fire(entry Entry) error
	Levels() []Level
}

// Entry 表示一条完整的日志记录。
type Entry struct {
	Time    time.Time
	Level   Level
	Message string
	Fields  []Field
}

// Formatter 定义日志格式化器的最小能力。
type Formatter interface {
	Format(entry Entry) ([]byte, error)
}

// Writer 定义日志输出端的最小写入与关闭能力。
type Writer interface {
	Write(p []byte) (n int, err error)
	Close() error
}

// Config 描述 logger integration 的初始化配置。
type Config struct {
	Name        string
	Level       Level
	Driver      string
	Format      string
	Output      OutputConfig
	Hooks       []Hook
	ContextKeys []string
	Formatter   Formatter
	Writer      Writer
}

// OutputConfig 描述日志输出端配置。
type OutputConfig struct {
	Type       string
	Filename   string
	MaxSize    int
	MaxBackups int
	MaxAge     int
	Compress   bool
	Writers    []io.Writer
}

// Factory 约定日志工厂需要提供的能力。
type Factory interface {
	Default() Logger
	New(cfg Config) (Logger, error)
}
