# lania-g v3 Auth 接入

## 1. 提供能力

`integrations/auth` 当前提供：

- JWT
- Session
- API Key
- RBAC
- Tenant Context
- `Guard + Binding` 注入用户信息

## 2. 基本接入

```go
authModule, err := auth.ForRoot(auth.Config{
	JWTSecret: "secret",
	APIKeys: map[string]auth.Principal{
		"internal-key": {Subject: "internal", Roles: []string{"admin"}},
	},
})
if err != nil {
	panic(err)
}

root := module.CreateModule([]module.Module{authModule}, nil, []any{controller}, nil, nil)
app, _ := application.NewWithOptions(root, application.Options{Registry: application.NewRegistry()}, httpAdapter)
authModuleValue := authModule.(*auth.Module)
auth.Install(app, authModuleValue.Service())
```

## 3. 推荐用法

- 全局安装认证 middleware
- 需要鉴权的 handler 使用 `auth.RequireAuthenticated(...)`
- 需要角色校验的 handler 使用 `auth.RequireRoles(...)`
- 需要租户上下文的 handler 使用 `auth.RequireTenant(...)`

## 4. 参数注入

支持以下绑定包装器：

- `auth.InjectPrincipal`
- `auth.InjectTenant`
- `auth.InjectClaims`
- `auth.PrincipalRef[T]`

## 5. 高级特性

### 5.1 更丰富的 Claims 校验

现在支持自定义 claims 校验器：

```go
authModule, err := auth.ForRoot(auth.Config{
    JWTSecret: "secret",
    ClaimsValidators: map[string]auth.ClaimsValidator{
        "age": func(claims map[string]any) error {
            if age, ok := claims["age"].(float64); ok {
                if age < 18 {
                    return fmt.Errorf("age must be at least 18")
                }
            }
            return nil
        },
        "email": func(claims map[string]any) error {
            if email, ok := claims["email"].(string); ok {
                if !strings.Contains(email, "@") {
                    return fmt.Errorf("invalid email format")
                }
            }
            return nil
        },
    },
})
```

也可以使用内置的校验器：

```go
validators := auth.ClaimsValidators()
authModule, err := auth.ForRoot(auth.Config{
    JWTSecret:        "secret",
    ClaimsValidators: validators,
})
```

### 5.2 策略组合

现在支持更灵活的策略组合：

```go
// 要求任何一个角色
auth.RequireAnyRole(service, "admin", "user")

// 要求所有角色
auth.RequireAllRoles(service, "admin", "manager")

// 链式组合多个守卫
auth.ChainGuards(
    auth.RequireAuthenticated(service),
    auth.RequireRoles(service, "admin"),
    auth.RequireClaims(service, map[string]interface{}{"active": true}),
)
```

### 5.3 外部 Identity Provider 支持

现在支持集成外部身份提供商：

```go
// 创建外部身份提供商
provider := auth.NewExampleIdentityProvider("google", "oauth2")

// 注册身份提供商
authService := authModuleValue.Service()
authService.RegisterIdentityProvider("google", provider)

// 使用外部身份提供商进行认证
// 在请求头中使用：Authorization: google <token>
```

### 5.4 更多守卫

- `auth.RequireClaims`：验证特定的 claims
- `auth.RequireAnyRole`：验证任何一个角色
- `auth.RequireAllRoles`：验证所有角色
- `auth.ChainGuards`：链式组合多个守卫
