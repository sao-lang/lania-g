// provider.go 实现 auth 集成的 provider 组装与派生能力。
package auth

import "time"

// ExampleIdentityProvider 是一个用于演示的外部身份提供方实现。
type ExampleIdentityProvider struct {
	name  string
	type_ string
}

// NewExampleIdentityProvider 创建一个示例身份提供方。
func NewExampleIdentityProvider(name, type_ string) *ExampleIdentityProvider {
	return &ExampleIdentityProvider{
		name:  name,
		type_: type_,
	}
}

// Authenticate 根据给定 token 返回一个示例 principal。
func (p *ExampleIdentityProvider) Authenticate(token string) (*Principal, error) {
	return &Principal{
		Subject:   "external-user",
		TenantID:  "external-tenant",
		Roles:     []string{"external-user"},
		Claims:    map[string]any{"provider": p.name},
		AuthType:  p.type_,
		Token:     token,
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}, nil
}

// Name 返回身份提供方名称。
func (p *ExampleIdentityProvider) Name() string {
	return p.name
}

// Type 返回身份提供方类型。
func (p *ExampleIdentityProvider) Type() string {
	return p.type_
}
