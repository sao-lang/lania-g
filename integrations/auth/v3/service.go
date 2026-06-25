// service.go 实现 auth 集成的核心服务封装与业务入口。
package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

// Current 返回当前 handler 上下文中的认证主体。
func Current(hc *runtime.HandlerContext) *Principal {
	if hc == nil {
		return nil
	}
	if value, ok := hc.Get(MetadataKeyPrincipal); ok {
		if principal, ok := value.(*Principal); ok {
			return principal
		}
	}
	if value, ok := hc.Context().Value(ContextKeyPrincipal).(*Principal); ok {
		return value
	}
	return nil
}

// CurrentTenant 返回当前 handler 上下文中的租户标识。
func CurrentTenant(hc *runtime.HandlerContext) string {
	if hc == nil {
		return ""
	}
	if value, ok := hc.Get(MetadataKeyTenant); ok {
		if tenant, ok := value.(string); ok {
			return tenant
		}
	}
	if value, ok := hc.Context().Value(ContextKeyTenant).(string); ok {
		return value
	}
	if principal := Current(hc); principal != nil {
		return principal.TenantID
	}
	return ""
}

func ensurePrincipal(service *Service, hc *runtime.HandlerContext) (*Principal, error) {
	if principal := Current(hc); principal != nil {
		return principal, nil
	}
	if service == nil {
		return nil, nil
	}
	principal, err := service.Authenticate(hc)
	if err != nil {
		return nil, err
	}
	if principal != nil {
		applyPrincipal(hc, principal)
	}
	return principal, nil
}

// Authenticate 从请求中提取凭证并解析出当前认证主体。
func (s *Service) Authenticate(hc *runtime.HandlerContext) (*Principal, error) {
	if hc == nil {
		return nil, nil
	}
	token, authType := s.findCredential(hc)
	if token == "" {
		tenant := firstValue(hc, s.config.TenantHeader, MetadataKeyTenant)
		if tenant != "" {
			principal := &Principal{TenantID: tenant, AuthType: "tenant"}
			applyPrincipal(hc, principal)
			return principal, nil
		}
		return nil, nil
	}
	var principal *Principal
	var err error
	switch authType {
	case "jwt":
		principal, err = s.authenticateJWT(token)
	case "apikey":
		principal, err = s.authenticateAPIKey(token)
	case "session":
		principal, err = s.authenticateSession(token)
	default:
		if provider := s.GetIdentityProvider(authType); provider != nil {
			principal, err = provider.Authenticate(token)
		} else {
			return nil, fmt.Errorf("unsupported auth type: %s", authType)
		}
	}
	if err != nil {
		return nil, err
	}
	if principal != nil && principal.TenantID == "" {
		principal.TenantID = firstValue(hc, s.config.TenantHeader, MetadataKeyTenant)
	}
	return principal, nil
}

func (s *Service) findCredential(hc *runtime.HandlerContext) (string, string) {
	if auth := firstValue(hc, s.config.AuthorizationHeader, "authorization"); auth != "" {
		parts := strings.Fields(auth)
		if len(parts) == 2 {
			switch strings.ToLower(parts[0]) {
			case "bearer":
				return parts[1], "jwt"
			case "apikey":
				return parts[1], "apikey"
			case "session":
				return parts[1], "session"
			}
		}
	}
	if value := firstValue(hc, s.config.APIKeyHeader, "auth.api_key"); value != "" {
		return value, "apikey"
	}
	if value := firstValue(hc, s.config.SessionHeader, "auth.session_id"); value != "" {
		return value, "session"
	}
	return "", ""
}

func firstValue(hc *runtime.HandlerContext, headerName string, metadataKey string) string {
	if hc == nil {
		return ""
	}
	for key, value := range hc.Request.Headers {
		if strings.EqualFold(key, headerName) {
			return value
		}
	}
	for key, values := range hc.Request.HeadersMulti {
		if strings.EqualFold(key, headerName) && len(values) > 0 {
			return values[0]
		}
	}
	if metadataKey != "" {
		if value, ok := hc.Get(metadataKey); ok {
			if s, ok := value.(string); ok {
				return s
			}
		}
	}
	return ""
}

func applyPrincipal(hc *runtime.HandlerContext, principal *Principal) {
	if hc == nil || principal == nil {
		return
	}
	hc.Set(MetadataKeyPrincipal, principal)
	hc.Set(MetadataKeyTenant, principal.TenantID)
	ctx := hc.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, ContextKeyPrincipal, principal)
	ctx = context.WithValue(ctx, ContextKeyTenant, principal.TenantID)
	hc.WithContext(ctx)
}

func (s *Service) authenticateAPIKey(key string) (*Principal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	principal, ok := s.config.APIKeys[key]
	if !ok {
		return nil, fmt.Errorf("invalid api key")
	}
	copyPrincipal := clonePrincipal(principal)
	copyPrincipal.AuthType = "apikey"
	copyPrincipal.Token = key
	return &copyPrincipal, nil
}

func (s *Service) authenticateSession(token string) (*Principal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	principal, ok := s.config.Sessions[token]
	if !ok {
		return nil, fmt.Errorf("invalid session")
	}
	copyPrincipal := clonePrincipal(principal)
	copyPrincipal.AuthType = "session"
	copyPrincipal.Token = token
	return &copyPrincipal, nil
}
