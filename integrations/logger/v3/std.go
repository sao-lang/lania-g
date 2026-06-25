// std.go 实现 logger 集成与标准库日志接口之间的适配。
package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

// DefaultConfig 返回一份默认日志配置。
func DefaultConfig() Config {
	return Config{
		Name:   "default",
		Level:  InfoLevel,
		Driver: "std",
		Format: "console",
		Output: OutputConfig{Type: "console"},
	}
}

// DevelopmentConfig 返回适合开发环境的日志配置。
func DevelopmentConfig() Config {
	cfg := DefaultConfig()
	cfg.Level = DebugLevel
	return cfg
}

// ProductionConfig 返回适合生产环境的日志配置。
func ProductionConfig() Config {
	cfg := DefaultConfig()
	cfg.Format = "json"
	return cfg
}

type stdLogger struct {
	level       Level
	fields      []Field
	formatter   Formatter
	writer      io.Writer
	hooks       []Hook
	contextKeys []string
	mu          sync.Mutex
}

type consoleFormatter struct{}
type jsonFormatter struct{}

// New 根据配置创建一个标准日志实现。
func New(cfg Config) (Logger, error) {
	cfg = normalizeConfig(cfg)
	var formatter Formatter
	if cfg.Formatter != nil {
		formatter = cfg.Formatter
	} else if cfg.Format == "json" {
		formatter = &jsonFormatter{}
	} else {
		formatter = &consoleFormatter{}
	}

	var writer io.Writer
	if cfg.Writer != nil {
		writer = cfg.Writer
	} else {
		writer = os.Stdout
	}

	return &stdLogger{
		level:       cfg.Level,
		formatter:   formatter,
		writer:      writer,
		hooks:       cfg.Hooks,
		contextKeys: append([]string{}, cfg.ContextKeys...),
	}, nil
}

func normalizeConfig(cfg Config) Config {
	def := DefaultConfig()
	if cfg.Name == "" {
		cfg.Name = def.Name
	}
	if cfg.Driver == "" {
		cfg.Driver = def.Driver
	}
	if cfg.Format == "" {
		cfg.Format = def.Format
	}
	if cfg.Output.Type == "" {
		cfg.Output = def.Output
	}
	return cfg
}

// Default 返回当前 logger 本身，便于满足 Factory 风格接口。
func (l *stdLogger) Default() Logger                   { return l }
// New 以工厂风格创建一个新的 logger。
func (l *stdLogger) New(cfg Config) (Logger, error)    { return New(cfg) }
// Debug 输出一条 `DEBUG` 级别日志。
func (l *stdLogger) Debug(msg string, fields ...Field) { l.log(DebugLevel, msg, fields...) }
// Info 输出一条 `INFO` 级别日志。
func (l *stdLogger) Info(msg string, fields ...Field)  { l.log(InfoLevel, msg, fields...) }
// Warn 输出一条 `WARN` 级别日志。
func (l *stdLogger) Warn(msg string, fields ...Field)  { l.log(WarnLevel, msg, fields...) }
// Error 输出一条 `ERROR` 级别日志。
func (l *stdLogger) Error(msg string, fields ...Field) { l.log(ErrorLevel, msg, fields...) }
// Fatal 输出一条 `FATAL` 级别日志。
func (l *stdLogger) Fatal(msg string, fields ...Field) { l.log(FatalLevel, msg, fields...) }
// Debugf 以格式化字符串输出 `DEBUG` 日志。
func (l *stdLogger) Debugf(format string, args ...interface{}) {
	l.Debug(fmt.Sprintf(format, args...))
}
// Infof 以格式化字符串输出 `INFO` 日志。
func (l *stdLogger) Infof(format string, args ...interface{}) { l.Info(fmt.Sprintf(format, args...)) }
// Warnf 以格式化字符串输出 `WARN` 日志。
func (l *stdLogger) Warnf(format string, args ...interface{}) { l.Warn(fmt.Sprintf(format, args...)) }
// Errorf 以格式化字符串输出 `ERROR` 日志。
func (l *stdLogger) Errorf(format string, args ...interface{}) {
	l.Error(fmt.Sprintf(format, args...))
}
// Fatalf 以格式化字符串输出 `FATAL` 日志。
func (l *stdLogger) Fatalf(format string, args ...interface{}) {
	l.Fatal(fmt.Sprintf(format, args...))
}
// With 追加结构化字段并返回一个派生 logger。
func (l *stdLogger) With(fields ...Field) Logger {
	l.mu.Lock()
	defer l.mu.Unlock()
	cloned := &stdLogger{
		level:       l.level,
		fields:      append(append([]Field{}, l.fields...), fields...),
		formatter:   l.formatter,
		writer:      l.writer,
		hooks:       append([]Hook{}, l.hooks...),
		contextKeys: append([]string{}, l.contextKeys...),
	}
	return cloned
}
// WithContext 从上下文提取配置的 key 并追加到日志字段中。
func (l *stdLogger) WithContext(ctx context.Context) Logger {
	if len(l.contextKeys) == 0 {
		return l
	}
	fields := make([]Field, 0, len(l.contextKeys))
	for _, key := range l.contextKeys {
		if value := ctx.Value(key); value != nil {
			fields = append(fields, Any(key, value))
		}
	}
	return l.With(fields...)
}
// Sync 刷新日志输出；标准实现这里是空操作。
func (l *stdLogger) Sync() error { return nil }

func (l *stdLogger) log(level Level, msg string, fields ...Field) {
	if level < l.level {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := Entry{
		Time:    time.Now(),
		Level:   level,
		Message: msg,
		Fields:  append(append([]Field{}, l.fields...), fields...),
	}
	for _, hook := range l.hooks {
		for _, supported := range hook.Levels() {
			if supported == level {
				_ = hook.Fire(entry)
				break
			}
		}
	}
	b, err := l.formatter.Format(entry)
	if err != nil {
		log.Printf("logger formatter error: %v", err)
		return
	}
	if _, err := l.writer.Write(b); err != nil {
		log.Printf("logger write error: %v", err)
	}
}

// Format 把日志条目格式化为控制台文本。
func (f *consoleFormatter) Format(entry Entry) ([]byte, error) {
	return []byte(fmt.Sprintf("%s [%s] %s %v\n", entry.Time.Format(time.RFC3339), entry.Level.String(), entry.Message, fieldsToMap(entry.Fields))), nil
}

// Format 把日志条目格式化为 JSON。
func (f *jsonFormatter) Format(entry Entry) ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"time":    entry.Time.Format(time.RFC3339),
		"level":   entry.Level.String(),
		"message": entry.Message,
		"fields":  fieldsToMap(entry.Fields),
	})
}

func fieldsToMap(fields []Field) map[string]interface{} {
	out := make(map[string]interface{}, len(fields))
	for _, field := range fields {
		out[field.Key] = field.Value
	}
	return out
}
