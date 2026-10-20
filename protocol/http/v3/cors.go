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
	AllowOrigins []string
	// AllowOriginFunc 用于动态判定 origin（子域通配、正则等）。
	// 命中时会把**请求的实际 origin** 回显，而不是 "*"。
	AllowOriginFunc func(origin string) bool
	AllowMethods    []string
	AllowHeaders    []string
	// AllowAllHeaders 为 true 时，preflight 会回显客户端请求的 header。
	// 默认 false：AllowHeaders 为空就不下发 Allow-Headers，避免无限制放行。
	AllowAllHeaders  bool
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
//
// 注意：`Access-Control-Allow-Origin: *` 与 `Allow-Credentials: true` 不能同时出现，
// 否则浏览器会直接拒绝响应。开启 credentials 时通配配置会退化为回显具体 origin。
func (c *CorsConfig) allowedOrigin(origin string) string {
	if c == nil || origin == "" {
		return ""
	}
	if c.AllowOriginFunc != nil && c.AllowOriginFunc(origin) {
		return origin
	}
	for _, o := range c.AllowOrigins {
		if o == "*" {
			if c.AllowCredentials {
				return origin
			}
			return "*"
		}
		if strings.EqualFold(o, origin) {
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
	// 无论是否命中，都要告诉缓存层"响应随 Origin 变化"，否则多 origin 会串味。
	w.Header().Add("Vary", "Origin")

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
		w.Header().Add("Vary", "Access-Control-Request-Method")
		w.Header().Add("Vary", "Access-Control-Request-Headers")

		if len(cfg.AllowMethods) > 0 {
			w.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowMethods, ", "))
		}

		requestHeaders := r.Header.Get("Access-Control-Request-Headers")
		switch {
		case len(cfg.AllowHeaders) > 0:
			w.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowHeaders, ", "))
		case cfg.AllowAllHeaders && requestHeaders != "":
			// 仅在显式开启时才回显，避免默认放行任意请求头。
			w.Header().Set("Access-Control-Allow-Headers", requestHeaders)
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
