// field.go 定义 logger 集成的结构化字段模型与字段辅助函数。
package logger

import "time"

// Field 表示一条结构化日志字段。
type Field struct {
	Key   string
	Value interface{}
}

// Object 表示适合挂到日志字段中的对象字典。
type Object map[string]interface{}

// String 创建一个字符串字段。
func String(key, value string) Field             { return Field{Key: key, Value: value} }
// Int 创建一个整数字段。
func Int(key string, value int) Field            { return Field{Key: key, Value: value} }
// Int64 创建一个 int64 字段。
func Int64(key string, value int64) Field        { return Field{Key: key, Value: value} }
// Uint 创建一个 uint 字段。
func Uint(key string, value uint) Field          { return Field{Key: key, Value: value} }
// Uint64 创建一个 uint64 字段。
func Uint64(key string, value uint64) Field      { return Field{Key: key, Value: value} }
// Float64 创建一个浮点数字段。
func Float64(key string, value float64) Field    { return Field{Key: key, Value: value} }
// Bool 创建一个布尔字段。
func Bool(key string, value bool) Field          { return Field{Key: key, Value: value} }
// Any 创建一个任意值字段。
func Any(key string, value interface{}) Field    { return Field{Key: key, Value: value} }
// ObjectField 创建一个对象字段。
func ObjectField(key string, value Object) Field { return Field{Key: key, Value: value} }
// Error 把错误包装成标准 `error` 字段。
func Error(err error) Field {
	if err == nil {
		return String("error", "")
	}
	return String("error", err.Error())
}
// Duration 创建一个时长字段。
func Duration(key string, value time.Duration) Field { return String(key, value.String()) }
// Time 创建一个 RFC3339 格式的时间字段。
func Time(key string, value time.Time) Field         { return String(key, value.Format(time.RFC3339)) }
// Fields 直接把字段列表收束为切片，便于调用方组合。
func Fields(fields ...Field) []Field                 { return fields }
