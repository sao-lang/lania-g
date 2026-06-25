// cors.go 提供 HTTP adapter 的 CORS 配置与中间件辅助。
package http

import (
	"net/http"
	"strconv"
	"strings"
)

// CorsConfig 是 net/http 层的 CORS 配置，行为与 v2 基本对齐。
// 这里只处理标准 CORS header 与 preflight，不引入更复杂的路由级策略。
type CorsConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           int
}

// DefaultCorsConfig 返回一份默认 CORS 配置。
// 默认是偏宽松的开发友好配置，生产环境通常会显式收紧 origin/header 范围。
func DefaultCorsConfig() *CorsConfig {
	return &CorsConfig{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{},
		AllowCredentials: false,
		MaxAge:           86400,
	}
}

// allowedOrigin 判断当前请求 origin 是否允许通过，并返回最终要写回的 origin 值。
func (c *CorsConfig) allowedOrigin(origin string) string {
	if c == nil || origin == "" || len(c.AllowOrigins) == 0 {
		return ""
	}
	for _, o := range c.AllowOrigins {
		if o == "*" {
			return "*"
		}
		if o == origin {
			return origin
		}
	}
	return ""
}

// applyCORS 负责写 CORS 头，并在需要时直接处理 OPTIONS preflight。
// 返回 true 表示请求已经在这里被完整处理，后续 handler 不应再继续执行。
func applyCORS(w http.ResponseWriter, r *http.Request, cfg *CorsConfig) bool {
	if cfg == nil || w == nil || r == nil {
		return false
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	allowOrigin := cfg.allowedOrigin(origin)
	if allowOrigin == "" {
		return false
	}

	w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
	if cfg.AllowCredentials {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}

	// Preflight 请求不进入业务 handler，直接在这里给出协商结果即可。
	if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
		if len(cfg.AllowMethods) > 0 {
			w.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowMethods, ", "))
		}

		requestHeaders := r.Header.Get("Access-Control-Request-Headers")
		if requestHeaders != "" {
			if len(cfg.AllowHeaders) > 0 {
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowHeaders, ", "))
			} else {
				w.Header().Set("Access-Control-Allow-Headers", requestHeaders)
			}
		}

		if cfg.MaxAge > 0 {
			w.Header().Set("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
		}

		w.WriteHeader(http.StatusNoContent)
		return true
	}

	// 普通跨域请求只补充 expose headers，其余仍继续交给业务 handler。
	if len(cfg.ExposeHeaders) > 0 {
		w.Header().Set("Access-Control-Expose-Headers", strings.Join(cfg.ExposeHeaders, ", "))
	}

	return false
}
