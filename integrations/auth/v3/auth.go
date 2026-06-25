// auth.go 提供 auth 集成的核心鉴权上下文读取与辅助能力。
package auth

import (
	"sync"
	"time"
)

const (
	// DefaultName 是默认认证服务实例名。
	DefaultName = "default"
	// DefaultAuthorizationHeader 是默认 Bearer Token 请求头名。
	DefaultAuthorizationHeader = "Authorization"
	// DefaultAPIKeyHeader 是默认 API Key 请求头名。
	DefaultAPIKeyHeader = "X-API-Key"
	// DefaultSessionHeader 是默认会话标识请求头名。
	DefaultSessionHeader = "X-Session-Id"
	// DefaultTenantHeader 是默认租户标识请求头名。
	DefaultTenantHeader = "X-Tenant-Id"
	// MetadataKeyPrincipal 是在 runtime metadata 中写入 principal 的键。
	MetadataKeyPrincipal = "auth.principal"
	// MetadataKeyTenant 是在 runtime metadata 中写入 tenant 的键。
	MetadataKeyTenant = "auth.tenant"
	// ContextKeyPrincipal 是在 context 中写入 principal 的键。
	ContextKeyPrincipal ctxKey = "auth.principal"
	// ContextKeyTenant 是在 context 中写入 tenant 的键。
	ContextKeyTenant ctxKey = "auth.tenant"
)

type ctxKey string

// IdentityProviderConfig 描述外部身份提供方的接入配置。
type IdentityProviderConfig struct {
	Type         string
	Endpoint     string
	ClientID     string
	ClientSecret string
	Scopes       []string
	RedirectURI  string
}

// ClaimsValidator 定义对 claims 进行附加校验的函数签名。
type ClaimsValidator func(claims map[string]any) error

// Config 描述认证服务的初始化配置。
type Config struct {
	Name                string
	JWTSecret           string
	JWTIssuer           string
	JWTAudience         string
	AuthorizationHeader string
	APIKeyHeader        string
	SessionHeader       string
	TenantHeader        string
	APIKeys             map[string]Principal
	Sessions            map[string]Principal
	Clock               func() time.Time
	IdentityProviders   map[string]IdentityProviderConfig
	ClaimsValidators    map[string]ClaimsValidator
}

// Principal 表示认证完成后得到的当前主体信息。
type Principal struct {
	Subject   string
	TenantID  string
	Roles     []string
	Claims    map[string]any
	AuthType  string
	Token     string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// Service 是 auth integration 对外暴露的核心认证服务。
type Service struct {
	config    Config
	mu        sync.RWMutex
	providers map[string]IdentityProvider
}

// IdentityProvider 定义外部身份提供方需要实现的能力。
type IdentityProvider interface {
	Authenticate(token string) (*Principal, error)
	Name() string
	Type() string
}

// Factory 约定认证服务工厂需要提供的能力。
type Factory interface {
	Default() *Service
	New(cfg Config) (*Service, error)
}

// InjectPrincipal 表示注入当前认证主体的包装类型。
type InjectPrincipal struct{ *Principal }

// InjectTenant 表示注入当前租户标识的包装类型。
type InjectTenant struct{ Value string }

// InjectClaims 表示注入当前 claims 集合的包装类型。
type InjectClaims struct{ Value map[string]any }

// PrincipalNamer 约定命名 principal 引用的名称来源。
type PrincipalNamer interface {
	PrincipalName() string
}

// PrincipalRef 表示按名称注入 principal 的包装类型。
type PrincipalRef[N any] struct {
	*Principal
	_ *N
}

// DefaultConfig 返回一份可直接使用的默认认证配置。
func DefaultConfig() Config {
	return Config{
		Name:                DefaultName,
		AuthorizationHeader: DefaultAuthorizationHeader,
		APIKeyHeader:        DefaultAPIKeyHeader,
		SessionHeader:       DefaultSessionHeader,
		TenantHeader:        DefaultTenantHeader,
		Clock:               time.Now,
	}
}

func normalizeConfig(cfg Config) Config {
	def := DefaultConfig()
	if cfg.Name == "" {
		cfg.Name = def.Name
	}
	if cfg.AuthorizationHeader == "" {
		cfg.AuthorizationHeader = def.AuthorizationHeader
	}
	if cfg.APIKeyHeader == "" {
		cfg.APIKeyHeader = def.APIKeyHeader
	}
	if cfg.SessionHeader == "" {
		cfg.SessionHeader = def.SessionHeader
	}
	if cfg.TenantHeader == "" {
		cfg.TenantHeader = def.TenantHeader
	}
	if cfg.Clock == nil {
		cfg.Clock = def.Clock
	}
	if cfg.APIKeys == nil {
		cfg.APIKeys = map[string]Principal{}
	}
	if cfg.Sessions == nil {
		cfg.Sessions = map[string]Principal{}
	}
	if cfg.IdentityProviders == nil {
		cfg.IdentityProviders = map[string]IdentityProviderConfig{}
	}
	if cfg.ClaimsValidators == nil {
		cfg.ClaimsValidators = map[string]ClaimsValidator{}
	}
	return cfg
}

// New 根据配置创建一个认证服务。
func New(cfg Config) (*Service, error) {
	cfg = normalizeConfig(cfg)
	return &Service{
		config:    cfg,
		providers: make(map[string]IdentityProvider),
	}, nil
}

// RegisterIdentityProvider 注册一个外部身份提供方。
func (s *Service) RegisterIdentityProvider(name string, provider IdentityProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers[name] = provider
}

// GetIdentityProvider 按名称返回一个外部身份提供方。
func (s *Service) GetIdentityProvider(name string) IdentityProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.providers[name]
}

// Default 返回当前服务本身，便于满足 Factory 风格接口。
func (s *Service) Default() *Service { return s }

// New 以工厂风格创建一个新的认证服务。
func (s *Service) New(cfg Config) (*Service, error) { return New(cfg) }

// Config 返回当前认证服务使用的配置快照。
func (s *Service) Config() Config { return s.config }
