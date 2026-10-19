// http-demo 演示 HTTP 协议的可运行组合：
//
//	模块树 + DI + 校验 + 全局 AOP（Envelope / Auth）+ Swagger
//
// 运行：
//
//	cd cmd && go run ./http-demo
//
// 路由声明写在控制器的 Declare 方法里，因此自动归属于控制器所在的模块；
// 鉴权与响应整形通过全局 interceptor 完成（等价 NestJS 的 APP_INTERCEPTOR）。
package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"

	application "github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/cmd/internal/addr"
	swagger "github.com/sao-lang/lania-g/integrations/swagger/v3"
	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
	httpbinding "github.com/sao-lang/lania-g/protocol/http/v3/binding"
)

const (
	sessionKey = "demo.auth.account"
	tokenKey   = "demo.auth.token"
)

type apiEnvelope struct {
	Msg     string `json:"msg"`
	Code    int    `json:"code"`
	Data    any    `json:"data"`
	Success bool   `json:"success"`
}

type account struct {
	ID       int
	Username string
	Password string
	Token    string
}

type accountView struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

type user struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

type createUserRequest struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gte=0"`
}

type updateUserRequest struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gte=0"`
}

type loginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// Store 是演示用的内存存储，通过 StoreModule 以 provider 形式导出。
type Store struct {
	mu       sync.Mutex
	users    []user
	accounts []account
	next     int
}

func NewStore() *Store {
	return &Store{
		users: []user{
			{ID: 1, Name: "Alice", Email: "alice@example.com", Age: 20},
			{ID: 2, Name: "Bob", Email: "bob@example.com", Age: 26},
		},
		accounts: []account{
			{ID: 1, Username: "demo", Password: "demo1234", Token: "demo-token"},
		},
		next: 3,
	}
}

// Login 校验账号密码并返回 token。
func (s *Store) Login(username, password string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.accounts {
		if item.Username == username && item.Password == password {
			return item.Token, true
		}
	}
	return "", false
}

// AccountByToken 按 token 查账号。
func (s *Store) AccountByToken(token string) (account, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.accounts {
		if item.Token == token {
			return item, true
		}
	}
	return account{}, false
}

// ListUsers 返回全部用户。
func (s *Store) ListUsers() []user {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]user{}, s.users...)
}

// FindUser 按 ID 查用户。
func (s *Store) FindUser(id int) (user, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.users {
		if item.ID == id {
			return item, true
		}
	}
	return user{}, false
}

// CreateUser 创建用户。
func (s *Store) CreateUser(req createUserRequest) user {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := user{ID: s.next, Name: req.Name, Email: req.Email, Age: req.Age}
	s.next++
	s.users = append(s.users, item)
	return item
}

// UpdateUser 更新用户。
func (s *Store) UpdateUser(id int, req updateUserRequest) (user, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, item := range s.users {
		if item.ID == id {
			s.users[i].Name = req.Name
			s.users[i].Email = req.Email
			s.users[i].Age = req.Age
			return s.users[i], true
		}
	}
	return user{}, false
}

// DeleteUser 删除用户。
func (s *Store) DeleteUser(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, item := range s.users {
		if item.ID == id {
			s.users = append(s.users[:i], s.users[i+1:]...)
			return true
		}
	}
	return false
}

// StoreModule 把 Store 装配成可导入模块。
func StoreModule(store *Store) module.Module {
	token := reflect.TypeFor[*Store]()
	provider, _ := di.ProviderFromInstanceWithToken(token, store, di.Singleton)
	return module.New(module.Metadata{
		Providers: []*di.Provider{provider},
		Exports:   []any{token},
	})
}

// SystemController 演示公开的系统接口。
type SystemController struct{}

// Home 是公开的首页。
func (c *SystemController) Home() map[string]any {
	return map[string]any{
		"service": "lania-g http-demo",
		"routes": []string{
			"POST /auth/login",
			"GET  /auth/me",
			"GET  /users",
			"GET  /users/:id",
			"POST /users",
			"PUT  /users/:id",
			"DELETE /users/:id",
			"GET  /docs",
		},
	}
}

// Declare 声明系统路由。
func (c *SystemController) Declare(r *httpadapter.Routes) {
	r.Get("/", c.Home).
		Summary("Demo Home").
		Description("返回 demo 首页与可用路由列表。").
		Tags("system")
}

// AuthController 演示公开接口与“当前登录用户”接口。
type AuthController struct {
	Store *Store
}

type loginArgs struct {
	Ctx  httpbinding.Context
	Body httpbinding.Body[loginRequest]
}

// Login 校验账号密码并返回 token。
func (c *AuthController) Login(args loginArgs) (any, error) {
	req := args.Body.Value
	token, ok := c.Store.Login(req.Username, req.Password)
	if !ok {
		return nil, aop.UnauthorizedException("账号或密码错误")
	}
	return map[string]string{"token": token}, nil
}

type meArgs struct {
	Ctx httpbinding.Context
}

// Me 返回当前登录账号（由 authInterceptor 写入上下文）。
func (c *AuthController) Me(args meArgs) (any, error) {
	value, ok := args.Ctx.Get(sessionKey)
	if !ok {
		return nil, aop.UnauthorizedException("未登录")
	}
	acc, ok := value.(account)
	if !ok {
		return nil, aop.InternalServerErrorException("登录信息异常")
	}
	return accountView{ID: acc.ID, Username: acc.Username}, nil
}

// Declare 声明认证路由。
func (c *AuthController) Declare(r *httpadapter.Routes) {
	r.Prefix("/auth").
		Post("/login", c.Login).
		Summary("Login").
		Description("使用账号密码登录并返回 token。").
		Tags("auth").
		ErrorResponse(http.StatusUnauthorized, "账号或密码错误").
		ResponseEnvelope(apiEnvelope{}, "data").
		Get("/me", c.Me).
		Summary("Current Account").
		Tags("auth").
		Security("bearerAuth").
		ResponseEnvelope(apiEnvelope{}, "data")
}

// UserController 演示受保护接口。
type UserController struct {
	Store *Store
}

type listUsersArgs struct {
	Ctx httpbinding.Context
}

// List 返回用户列表。
func (c *UserController) List(listUsersArgs) (any, error) {
	return c.Store.ListUsers(), nil
}

type getUserArgs struct {
	Ctx httpbinding.Context
	ID  httpbinding.Param[int] `param:"id"`
}

// Get 返回单个用户。
func (c *UserController) Get(args getUserArgs) (any, error) {
	item, ok := c.Store.FindUser(args.ID.Value)
	if !ok {
		return nil, aop.NotFoundException("用户不存在")
	}
	return item, nil
}

type createUserArgs struct {
	Ctx  httpbinding.Context
	Body httpbinding.Body[createUserRequest]
}

// Create 创建用户。
func (c *UserController) Create(args createUserArgs) (any, error) {
	item := c.Store.CreateUser(args.Body.Value)
	args.Ctx.Status(http.StatusCreated)
	return item, nil
}

type updateUserArgs struct {
	Ctx  httpbinding.Context
	ID   httpbinding.Param[int] `param:"id"`
	Body httpbinding.Body[updateUserRequest]
}

// Update 更新用户。
func (c *UserController) Update(args updateUserArgs) (any, error) {
	item, ok := c.Store.UpdateUser(args.ID.Value, args.Body.Value)
	if !ok {
		return nil, aop.NotFoundException("用户不存在")
	}
	return item, nil
}

type deleteUserArgs struct {
	Ctx httpbinding.Context
	ID  httpbinding.Param[int] `param:"id"`
}

// Delete 删除用户。
func (c *UserController) Delete(args deleteUserArgs) (any, error) {
	if !c.Store.DeleteUser(args.ID.Value) {
		return nil, aop.NotFoundException("用户不存在")
	}
	return map[string]any{"deleted": args.ID.Value}, nil
}

// Declare 声明用户路由。
func (c *UserController) Declare(r *httpadapter.Routes) {
	r.Prefix("/users").
		Get("", c.List).
		Summary("List Users").
		Description("返回全部用户。").
		Tags("users").
		Security("bearerAuth").
		ResponseEnvelope(apiEnvelope{}, "data").
		Get("/:id", c.Get).
		Summary("Get User").
		Tags("users").
		Security("bearerAuth").
		ResponseEnvelope(apiEnvelope{}, "data").
		ErrorResponse(http.StatusNotFound, "用户不存在").
		Post("", c.Create).
		Summary("Create User").
		Tags("users").
		Security("bearerAuth").
		Status(http.StatusCreated).
		ErrorResponse(http.StatusBadRequest, "参数错误").
		ResponseEnvelope(apiEnvelope{}, "data").
		Put("/:id", c.Update).
		Summary("Update User").
		Tags("users").
		Security("bearerAuth").
		ResponseEnvelope(apiEnvelope{}, "data").
		ErrorResponse(http.StatusNotFound, "用户不存在").
		Delete("/:id", c.Delete).
		Summary("Delete User").
		Tags("users").
		Security("bearerAuth").
		ResponseEnvelope(apiEnvelope{}, "data").
		ErrorResponse(http.StatusNotFound, "用户不存在")
}

// responseEnvelopeInterceptor 统一成功/失败响应结构。
//
// 等价 NestJS 的全局 Interceptor：包裹 handler 执行，并把错误转换成统一 envelope。
func responseEnvelopeInterceptor(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
	result, err := next.Handle()
	hc, _ := ctx.HandlerContext.(*runtime.HandlerContext)

	if err != nil {
		status, msg := statusAndMessageFromError(err)
		if hc != nil {
			hc.Response.Status = status
		}
		return apiEnvelope{Msg: msg, Code: status, Data: nil, Success: false}, nil
	}

	if envelope, ok := result.(apiEnvelope); ok {
		return envelope, nil
	}

	status := http.StatusOK
	if hc != nil && hc.Response.Status > 0 {
		status = hc.Response.Status
	}
	return apiEnvelope{Msg: "ok", Code: status, Data: result, Success: true}, nil
}

// authInterceptor 校验 Bearer token，并把登录账号写入请求上下文。
//
// 公开路由（首页与登录）直接放行；其余路由必须携带有效 token。
func authInterceptor(store *Store) aop.InterceptorFunc {
	return func(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
		if isPublicRoute(ctx.RouteKey) {
			return next.Handle()
		}

		hc, ok := ctx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return nil, aop.InternalServerErrorException("request context unavailable")
		}

		token, err := extractBearerToken(hc.Request.Headers["Authorization"])
		if err != nil {
			return nil, err
		}
		acc, ok := store.AccountByToken(token)
		if !ok {
			return nil, aop.UnauthorizedException("登录已失效，请重新登录")
		}
		hc.Set(sessionKey, acc)
		hc.Set(tokenKey, token)
		return next.Handle()
	}
}

// isPublicRoute 判断当前路由是否允许匿名访问。
func isPublicRoute(routeKey string) bool {
	switch routeKey {
	case "http:GET:/", "http:POST:/auth/login":
		return true
	default:
		return false
	}
}

// extractBearerToken 从 Authorization 头解析 token。
func extractBearerToken(header string) (string, error) {
	value := strings.TrimSpace(header)
	if value == "" {
		return "", aop.UnauthorizedException("请在 Authorization 头中携带 Bearer token")
	}
	parts := strings.Fields(value)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", aop.UnauthorizedException("Authorization 格式应为 Bearer <token>")
	}
	if strings.TrimSpace(parts[1]) == "" {
		return "", aop.UnauthorizedException("token 不能为空")
	}
	return parts[1], nil
}

// statusAndMessageFromError 把内核错误 / HTTP 异常映射成状态码与文案。
func statusAndMessageFromError(err error) (int, string) {
	status := http.StatusInternalServerError
	msg := "服务器内部错误"

	var kerr *kerrors.KernelError
	if errors.As(err, &kerr) && kerr != nil {
		switch kerr.Kind {
		case kerrors.KindRouteNotFound:
			status = http.StatusNotFound
		case kerrors.KindForbidden:
			status = http.StatusForbidden
		case kerrors.KindUnauthorized:
			status = http.StatusUnauthorized
		case kerrors.KindBinding, kerrors.KindValidation, kerrors.KindDI:
			status = http.StatusBadRequest
		}
		if strings.TrimSpace(kerr.Error()) != "" {
			msg = kerr.Error()
		}
		return status, msg
	}

	var httpErr *aop.HttpException
	if errors.As(err, &httpErr) && httpErr != nil {
		if httpErr.Status > 0 {
			status = httpErr.Status
		}
		if strings.TrimSpace(httpErr.Message) != "" {
			msg = httpErr.Message
		}
		return status, msg
	}

	if strings.TrimSpace(err.Error()) != "" {
		msg = err.Error()
	}
	return status, msg
}

func main() {
	store := NewStore()

	httpModule := httpadapter.Module(httpadapter.Config{
		Addr:      addr.HTTP(":8080"),
		Validator: true,
	})
	swaggerModule := swagger.Module(
		swagger.Config{
			Title:                "lania-g HTTP Demo API",
			Description:          "模块树 + DI + AOP + Swagger",
			Version:              "1.0.0",
			BearerAuth:           []string{"bearerAuth"},
			DefaultErrorResponse: "请求失败",
		},
		&swagger.UIConfig{
			SpecURL:    "/docs/openapi.json",
			SwaggerURL: "/docs",
			RedocURL:   "/redoc",
		},
	)

	root := module.New(module.Metadata{
		Imports: []module.Module{
			httpModule,
			swaggerModule,
			StoreModule(store),
		},
		Controllers: []any{&SystemController{}, &AuthController{}, &UserController{}},
	})

	app, err := application.New(root, application.Options{Logger: os.Stdout})
	if err != nil {
		log.Fatal(err)
	}

	// 全局 AOP：先整形响应，再校验身份（顺序即执行顺序）。
	app.UseGlobalInterceptors(responseEnvelopeInterceptor, authInterceptor(store))

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
