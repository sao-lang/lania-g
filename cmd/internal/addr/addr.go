// Package addr 统一示例工程的监听地址解析。
//
// 约定：
// - 先读协议专用环境变量（APP_ADDR / GRPC_ADDR）
// - 再读通用的 PORT（只写数字时自动补 `:`）
// - 都没有时使用调用方给出的默认地址
package addr

import (
	"os"
	"strings"
)

// HTTP 返回 HTTP/WS/GraphQL 示例的监听地址。
func HTTP(defaultAddr string) string {
	return resolve(defaultAddr, "APP_ADDR", "PORT")
}

// GRPC 返回 gRPC 示例的监听地址。
func GRPC(defaultAddr string) string {
	return resolve(defaultAddr, "GRPC_ADDR", "PORT")
}

func resolve(defaultAddr string, keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		// 支持 `PORT=8080` 与 `APP_ADDR=:8080` / `APP_ADDR=127.0.0.1:8080` 两种写法。
		if !strings.Contains(value, ":") {
			value = ":" + value
		}
		return value
	}
	return defaultAddr
}
