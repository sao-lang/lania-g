// level.go 定义 logger 集成的日志级别语义与转换辅助。
package logger

import "strings"

// Level 表示日志级别。
type Level int

const (
	// DebugLevel 表示调试级别日志。
	DebugLevel Level = iota
	// InfoLevel 表示信息级别日志。
	InfoLevel
	// WarnLevel 表示警告级别日志。
	WarnLevel
	// ErrorLevel 表示错误级别日志。
	ErrorLevel
	// FatalLevel 表示致命级别日志。
	FatalLevel
)

// ParseLevel 把字符串解析为日志级别；无法识别时回退到 `InfoLevel`。
func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "debug":
		return DebugLevel
	case "info":
		return InfoLevel
	case "warn", "warning":
		return WarnLevel
	case "error":
		return ErrorLevel
	case "fatal":
		return FatalLevel
	default:
		return InfoLevel
	}
}

// String 返回日志级别对应的文本表示。
func (l Level) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	case FatalLevel:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}
