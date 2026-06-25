// jwt.go 实现 auth 集成使用的 JWT 编解码与校验辅助。
package auth

import (
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func (s *Service) authenticateJWT(token string) (*Principal, error) {
	if strings.TrimSpace(s.config.JWTSecret) == "" {
		return nil, fmt.Errorf("jwt secret is not configured")
	}
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		return []byte(s.config.JWTSecret), nil
	}, jwt.WithIssuer(s.config.JWTIssuer))
	if err != nil {
		parsed, err = jwt.Parse(token, func(t *jwt.Token) (any, error) {
			return []byte(s.config.JWTSecret), nil
		})
		if err != nil {
			return nil, err
		}
	}
	if !parsed.Valid {
		return nil, fmt.Errorf("invalid jwt token")
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("unsupported jwt claims")
	}

	for name, validator := range s.config.ClaimsValidators {
		if err := validator(claims); err != nil {
			return nil, fmt.Errorf("claims validation failed for %s: %w", name, err)
		}
	}

	principal := &Principal{
		Subject:  stringClaim(claims, "sub"),
		TenantID: stringClaim(claims, "tenant_id"),
		Roles:    rolesClaim(claims["roles"]),
		Claims:   mapClaims(claims),
		AuthType: "jwt",
		Token:    token,
	}
	if exp, ok := numericDate(claims["exp"]); ok {
		principal.ExpiresAt = exp
	}
	if iat, ok := numericDate(claims["iat"]); ok {
		principal.IssuedAt = iat
	}
	if principal.Subject == "" {
		principal.Subject = stringClaim(claims, "user_id")
	}
	return principal, nil
}

func clonePrincipal(in Principal) Principal {
	out := in
	out.Roles = append([]string{}, in.Roles...)
	if in.Claims != nil {
		out.Claims = mapClaims(in.Claims)
	}
	return out
}

func mapClaims(claims jwt.MapClaims) map[string]any {
	out := make(map[string]any, len(claims))
	maps.Copy(out, claims)
	return out
}

func stringClaim(claims jwt.MapClaims, key string) string {
	if value, ok := claims[key]; ok {
		switch v := value.(type) {
		case string:
			return v
		case fmt.Stringer:
			return v.String()
		}
	}
	return ""
}

func rolesClaim(value any) []string {
	switch v := value.(type) {
	case []string:
		return append([]string{}, v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if v == "" {
			return nil
		}
		return strings.Split(v, ",")
	default:
		return nil
	}
}

func numericDate(value any) (time.Time, bool) {
	switch v := value.(type) {
	case float64:
		return time.Unix(int64(v), 0), true
	case int64:
		return time.Unix(v, 0), true
	case jsonNumber:
		if parsed, err := v.Int64(); err == nil {
			return time.Unix(parsed, 0), true
		}
	}
	return time.Time{}, false
}

type jsonNumber interface {
	Int64() (int64, error)
}

// ClaimsValidators 返回内置的 JWT claims 校验器集合。
func ClaimsValidators() map[string]ClaimsValidator {
	return map[string]ClaimsValidator{
		"exp": func(claims map[string]any) error {
			if exp, ok := claims["exp"]; ok {
				expTime, ok := numericDate(exp)
				if !ok {
					return fmt.Errorf("invalid exp claim")
				}
				if time.Now().After(expTime) {
					return fmt.Errorf("token expired")
				}
			}
			return nil
		},
		"iat": func(claims map[string]any) error {
			if iat, ok := claims["iat"]; ok {
				_, ok := numericDate(iat)
				if !ok {
					return fmt.Errorf("invalid iat claim")
				}
			}
			return nil
		},
		"sub": func(claims map[string]any) error {
			if sub, ok := claims["sub"]; !ok || sub == "" {
				return fmt.Errorf("missing or empty sub claim")
			}
			return nil
		},
	}
}
