// http-demo 演示 HTTP 协议的可运行组合：
//
//	模块树 + DI + 校验 + 全局 Guard/Interceptor + Swagger
//
// 运行：
//
//	cd cmd && PORT=18080 go run ./http-demo
//
// 路由与鉴权声明都在控制器内部完成（Register），因此天然归属于所在模块。
package main

import (
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
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
	httpbinding "github.com/sao-lang/lania-g/protocol/http/v3/binding"
)

const sessionKey = "demo.auth.account"

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

// CreateUser 创建用户。
func (s *Store) CreateUser(req createUserRequest) user {
	s.mu.Lock()
	defer s.mu.Unlock()
	item := user{ID: s.next, Name: req.Name, Email: req.Email, Age: req.Age}
	s.next++
	s.users = append(s.users, item)
	return item
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

// AuthController 演示公开接口。
type AuthController struct{}

type loginArgs struct {
	Ctx  httpbinding.Context
	Body httpbinding.Body[loginRequest]
}

// Login 换取 token。
func (c *AuthController) Login(args loginArgs) (any, error) {
	return map[string]string{"token": "demo-token"}, nil
}

// Register 声明认证路由。
func (c *AuthController) Register(r *httpadapter.Routes) {
	r.Prefix("/auth").
		Post("/login", c.Login).
		Summary("Login").
		Tags("auth").
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

type createUserArgs struct {
	Ctx  httpbinding.Context
	Body httpbinding.Body[createUserRequest]
}

// Create 创建用户。
func (c *UserController) Create(args createUserArgs) (any, error) {
	return c.Store.CreateUser(args.Body.Value), nil
}

// Register 声明用户路由。
func (c *UserController) Register(r *httpadapter.Routes) {
	r.Prefix("/users").
		UseGuards(requireBearerToken()).
		Get("", c.List).
		Summary("List Users").
		Tags("users").
		Security("bearerAuth").
		ResponseEnvelope(apiEnvelope{}, "data").
		Post("", c.Create).
		Summary("Create User").
		Tags("users").
		Security("bearerAuth").
		Status(http.StatusCreated).
		ErrorResponse(http.StatusBadRequest, "参数错误").
		ResponseEnvelope(apiEnvelope{}, "data")
}

// requireBearerToken 是一个不依赖外部实例的 guard：直接从请求头取 token。
func requireBearerToken() aop.GuardFunc {
	return func(ctx *aop.ExecutionContext) (bool, error) {
		hc, ok := ctx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return false, aop.UnauthorizedException("无法读取请求上下文")
		}
		token := strings.TrimSpace(strings.TrimPrefix(hc.Request.Headers["Authorization"], "Bearer"))
		if token == "" {
			return false, aop.UnauthorizedException("请在 Authorization 头中携带 Bearer token")
		}
		hc.Set(sessionKey, token)
		return true, nil
	}
}

// envelopeInterceptor 统一包装成功响应。
func envelopeInterceptor(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
	result, err := next.Handle()
	if err != nil {
		return nil, err
	}
	if _, ok := result.(apiEnvelope); ok {
		return result, nil
	}
	return apiEnvelope{Msg: "ok", Code: http.StatusOK, Data: result, Success: true}, nil
}

func main() {
	store := NewStore()

	httpModule := httpadapter.Module(httpadapter.Config{
		Addr:      addr.HTTP(":8080"),
		Validator: true,
	})
	swaggerModule := swagger.Module(
		swagger.Config{
			Title:       "lania-g HTTP Demo API",
			Description: "模块树 + DI + AOP + Swagger",
			Version:     "1.0.0",
			BearerAuth:  []string{"bearerAuth"},
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
		Controllers: []any{&AuthController{}, &UserController{}},
	})

	app, err := application.New(root, application.Options{Logger: os.Stdout})
	if err != nil {
		log.Fatal(err)
	}
	app.UseGlobalInterceptors(envelopeInterceptor)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
