// helmet.go 提供 HTTP adapter 的安全响应头辅助。
package http

import (
	"net/http"
	"strings"
)

// HelmetConfig 是 net/http 层的安全响应头配置，行为与 v2 基本对齐。
// 它只负责“写哪些安全头”，不做请求拦截或内容改写。
type HelmetConfig struct {
	ContentSecurityPolicy         string
	XContentTypeOptions           string
	XFrameOptions                 string
	XSSProtection                 string
	StrictTransportSecurity       string
	ReferrerPolicy                string
	PermissionsPolicy             string
	XDNSPrefetchControl           string
	XDownloadOptions              string
	XPermittedCrossDomainPolicies string
}

// DefaultHelmetConfig 返回一份默认的安全响应头配置。
// 默认值偏向传统 Web 安全头的保守配置，可按实际应用能力再覆盖。
func DefaultHelmetConfig() *HelmetConfig {
	return &HelmetConfig{
		XContentTypeOptions:           "nosniff",
		XFrameOptions:                 "DENY",
		XSSProtection:                 "1; mode=block",
		StrictTransportSecurity:       "max-age=15552000; includeSubDomains",
		ReferrerPolicy:                "no-referrer",
		XDNSPrefetchControl:           "off",
		XDownloadOptions:              "noopen",
		XPermittedCrossDomainPolicies: "none",
	}
}

// applyHelmet 把配置里的安全响应头写回当前响应。
// 大多数头无条件写入；`Strict-Transport-Security` 只在 HTTPS 请求上生效。
func applyHelmet(w http.ResponseWriter, r *http.Request, cfg *HelmetConfig) {
	if cfg == nil || w == nil || r == nil {
		return
	}
	if cfg.ContentSecurityPolicy != "" {
		w.Header().Set("Content-Security-Policy", cfg.ContentSecurityPolicy)
	}
	if cfg.XContentTypeOptions != "" {
		w.Header().Set("X-Content-Type-Options", cfg.XContentTypeOptions)
	}
	if cfg.XFrameOptions != "" {
		w.Header().Set("X-Frame-Options", cfg.XFrameOptions)
	}
	if cfg.XSSProtection != "" {
		w.Header().Set("X-XSS-Protection", cfg.XSSProtection)
	}
	if cfg.StrictTransportSecurity != "" && isHTTPS(r) {
		w.Header().Set("Strict-Transport-Security", cfg.StrictTransportSecurity)
	}
	if cfg.ReferrerPolicy != "" {
		w.Header().Set("Referrer-Policy", cfg.ReferrerPolicy)
	}
	if cfg.PermissionsPolicy != "" {
		w.Header().Set("Permissions-Policy", cfg.PermissionsPolicy)
	}
	if cfg.XDNSPrefetchControl != "" {
		w.Header().Set("X-DNS-Prefetch-Control", cfg.XDNSPrefetchControl)
	}
	if cfg.XDownloadOptions != "" {
		w.Header().Set("X-Download-Options", cfg.XDownloadOptions)
	}
	if cfg.XPermittedCrossDomainPolicies != "" {
		w.Header().Set("X-Permitted-Cross-Domain-Policies", cfg.XPermittedCrossDomainPolicies)
	}
}

// isHTTPS 同时兼容直连 TLS 和经过反向代理转发 `X-Forwarded-Proto: https` 的场景。
func isHTTPS(r *http.Request) bool {
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
