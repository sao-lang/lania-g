// http-demo 是 HTTP 集成的能力演示。
// 它不是推荐的业务项目模板，业务接入优先参考 `docs/标准接入模板.md`。
package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/sao-lang/lania-g/application/v3"
	swagger "github.com/sao-lang/lania-g/integrations/swagger/v3"
	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
	httpbinding "github.com/sao-lang/lania-g/protocol/http/v3/binding"
)

const (
	authAccountKey = "demo.auth.account"
	authTokenKey   = "demo.auth.token"
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

type session struct {
	Token     string
	AccountID int
	Username  string
}

type memoryStore struct {
	mu            sync.Mutex
	accounts      []account
	users         []user
	sessions      []session
	nextAccountID int
	nextUserID    int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		accounts:      make([]account, 0),
		users:         []user{{ID: 1, Name: "Alice", Email: "alice@example.com", Age: 20}, {ID: 2, Name: "Bob", Email: "bob@example.com", Age: 26}},
		sessions:      make([]session, 0),
		nextAccountID: 1,
		nextUserID:    3,
	}
}

func (s *memoryStore) register(username, password string) (accountView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, item := range s.accounts {
		if strings.EqualFold(item.Username, username) {
			return accountView{}, aop.ConflictException("账号已存在")
		}
	}

	acc := account{
		ID:       s.nextAccountID,
		Username: username,
		Password: password,
	}
	s.nextAccountID++
	s.accounts = append(s.accounts, acc)
	return accountView{ID: acc.ID, Username: acc.Username}, nil
}

func (s *memoryStore) login(username, password string) (string, accountView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, item := range s.accounts {
		if strings.EqualFold(item.Username, username) && item.Password == password {
			token := fmt.Sprintf("token-%d-%d", item.ID, time.Now().UnixNano())
			s.sessions = append(s.sessions, session{
				Token:     token,
				AccountID: item.ID,
				Username:  item.Username,
			})
			return token, accountView{ID: item.ID, Username: item.Username}, nil
		}
	}

	return "", accountView{}, aop.UnauthorizedException("账号或密码错误")
}

func (s *memoryStore) findAccountByToken(token string) (accountView, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, sess := range s.sessions {
		if sess.Token == token {
			return accountView{ID: sess.AccountID, Username: sess.Username}, true
		}
	}
	return accountView{}, false
}

func (s *memoryStore) createUser(name, email string, age int) user {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := user{
		ID:    s.nextUserID,
		Name:  name,
		Email: email,
		Age:   age,
	}
	s.nextUserID++
	s.users = append(s.users, item)
	return item
}

func (s *memoryStore) listUsers(keyword string, page, size int) ([]user, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := make([]user, 0, len(s.users))
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	for _, item := range s.users {
		if keyword == "" || strings.Contains(strings.ToLower(item.Name), keyword) || strings.Contains(strings.ToLower(item.Email), keyword) {
			filtered = append(filtered, item)
		}
	}

	total := len(filtered)
	start := (page - 1) * size
	if start >= total {
		return []user{}, total
	}
	end := start + size
	if end > total {
		end = total
	}
	out := make([]user, end-start)
	copy(out, filtered[start:end])
	return out, total
}

func (s *memoryStore) getUserByID(id int) (user, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, item := range s.users {
		if item.ID == id {
			return item, nil
		}
	}
	return user{}, aop.NotFoundException("用户不存在")
}

func (s *memoryStore) updateUser(id int, name, email string, age int) (user, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.users {
		if s.users[i].ID == id {
			s.users[i].Name = name
			s.users[i].Email = email
			s.users[i].Age = age
			return s.users[i], nil
		}
	}
	return user{}, aop.NotFoundException("用户不存在")
}

func (s *memoryStore) deleteUser(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.users {
		if s.users[i].ID == id {
			s.users = append(s.users[:i], s.users[i+1:]...)
			return nil
		}
	}
	return aop.NotFoundException("用户不存在")
}

// AuthController 演示一个简单的认证相关 Controller（注册/登录）。
type AuthController struct {
	Store *memoryStore // 注入点：由模块 Init 时自动从容器注入
}

// UserController 演示一个简单的用户管理 Controller（CRUD + 列表）。
type UserController struct {
	Store *memoryStore // 注入点：由模块 Init 时自动从容器注入
}

type registerRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type createUserRequest struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gt=0"`
}

type updateUserRequest struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gt=0"`
}

type registerArgs struct {
	Ctx  httpbinding.Context
	Body httpbinding.Body[registerRequest]
}

type loginArgs struct {
	Ctx  httpbinding.Context
	Body httpbinding.Body[loginRequest]
}

type listUsersArgs struct {
	Ctx     httpbinding.Context
	Page    httpbinding.Query[int]     `query:"page"`
	Size    httpbinding.Query[int]     `query:"size"`
	Keyword httpbinding.Query[string]  `query:"keyword"`
	TraceID httpbinding.Header[string] `header:"X-Trace-Id"`
}

type getUserArgs struct {
	ID      httpbinding.Param[int]     `param:"id" required:"true"`
	TraceID httpbinding.Header[string] `header:"X-Trace-Id"`
}

type createUserArgs struct {
	Ctx  httpbinding.Context
	Body httpbinding.Body[createUserRequest]
}

type updateUserArgs struct {
	Ctx  httpbinding.Context
	ID   httpbinding.Param[int]        `param:"id" required:"true"`
	Body httpbinding.Body[updateUserRequest]
}

type deleteUserArgs struct {
	Ctx httpbinding.Context
	ID  httpbinding.Param[int] `param:"id" required:"true"`
}

// Home 返回 demo 首页，方便直接在浏览器确认服务状态。
func (c *AuthController) Home() any {
	return map[string]any{
		"name":    "lania-g http demo",
		"message": "service is running",
		"routes": []string{
			"GET /",
			"GET /docs",
			"GET /docs/openapi.json",
			"POST /auth/register",
			"POST /auth/login",
			"GET /users",
			"GET /users/:id",
			"POST /users",
			"PUT /users/:id",
			"DELETE /users/:id",
		},
	}
}

// Register 注册账号，并在成功时返回 201。
func (c *AuthController) Register(args registerArgs) (any, error) {
	if err := validateCredentials(args.Body.Value.Username, args.Body.Value.Password); err != nil {
		return nil, err
	}

	account, err := c.Store.register(args.Body.Value.Username, args.Body.Value.Password)
	if err != nil {
		return nil, err
	}

	// 保持 HTTP 语义：注册操作会创建资源，因此返回 201。
	args.Ctx.Status(http.StatusCreated)
	return map[string]any{"account": account}, nil
}

// Login 登录并返回 token 与账号信息。
func (c *AuthController) Login(args loginArgs) (any, error) {
	if err := validateCredentials(args.Body.Value.Username, args.Body.Value.Password); err != nil {
		return nil, err
	}

	token, account, err := c.Store.login(args.Body.Value.Username, args.Body.Value.Password)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"token":   token,
		"account": account,
	}, nil
}

// List 返回用户列表（带分页参数与关键字过滤），并回显 trace id 与当前账号信息。
func (c *UserController) List(args listUsersArgs) (any, error) {
	page := args.Page.Value
	size := args.Size.Value
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 10
	}

	items, total := c.Store.listUsers(args.Keyword.Value, page, size)
	account, _ := currentAccount(args.Ctx)
	return map[string]any{
		"list":     items,
		"total":    total,
		"page":     page,
		"size":     size,
		"keyword":  args.Keyword.Value,
		"trace_id": args.TraceID.Value,
		"account":  account,
	}, nil
}

// Get 按 id 获取一个用户。
func (c *UserController) Get(args getUserArgs) (any, error) {
	item, err := c.Store.getUserByID(args.ID.Value)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"user":     item,
		"trace_id": args.TraceID.Value,
	}, nil
}

// Create 创建一个用户，并在成功时返回 201。
func (c *UserController) Create(args createUserArgs) (any, error) {
	req := args.Body.Value
	item := c.Store.createUser(req.Name, req.Email, req.Age)
	args.Ctx.Status(http.StatusCreated)
	return item, nil
}

// Update 按 id 更新一个用户。
func (c *UserController) Update(args updateUserArgs) (any, error) {
	req := args.Body.Value
	item, err := c.Store.updateUser(args.ID.Value, req.Name, req.Email, req.Age)
	if err != nil {
		return nil, err
	}

	return item, nil
}

// Delete 按 id 删除一个用户。
func (c *UserController) Delete(args deleteUserArgs) (any, error) {
	if err := c.Store.deleteUser(args.ID.Value); err != nil {
		return nil, err
	}

	return map[string]any{"deleted_id": args.ID.Value}, nil
}

func validateCredentials(username, password string) error {
	if strings.TrimSpace(username) == "" {
		return aop.BadRequestException("username 不能为空")
	}
	if len(strings.TrimSpace(password)) < 6 {
		return aop.BadRequestException("password 长度不能少于 6 位")
	}
	return nil
}

func responseEnvelopeInterceptor(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
	result, err := next.Handle()
	hc, _ := ctx.HandlerContext.(*runtime.HandlerContext)

	if err != nil {
		status, msg := statusAndMessageFromError(err)
		if hc != nil {
			hc.Response.Status = status
		}
		return apiEnvelope{
			Msg:     msg,
			Code:    status,
			Data:    nil,
			Success: false,
		}, nil
	}

	if envelope, ok := result.(apiEnvelope); ok {
		return envelope, nil
	}

	status := http.StatusOK
	if hc != nil && hc.Response.Status > 0 {
		status = hc.Response.Status
	}

	msg := "ok"
	data := result

	return apiEnvelope{
		Msg:     msg,
		Code:    status,
		Data:    data,
		Success: true,
	}, nil
}

func authInterceptor(store *memoryStore) aop.InterceptorFunc {
	return func(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
		if isPublicRoute(ctx.RouteKey) {
			return next.Handle()
		}

		hc, _ := ctx.HandlerContext.(*runtime.HandlerContext)
		if hc == nil {
			return nil, aop.InternalServerErrorException("request context unavailable")
		}

		token, err := extractBearerToken(hc.Request.Headers["Authorization"])
		if err != nil {
			return nil, err
		}

		account, ok := store.findAccountByToken(token)
		if !ok {
			return nil, aop.UnauthorizedException("登录已失效，请重新登录")
		}

		hc.Set(authAccountKey, account)
		hc.Set(authTokenKey, token)
		return next.Handle()
	}
}

func isPublicRoute(routeKey string) bool {
	switch routeKey {
	case "http:GET:/", "http:POST:/auth/register", "http:POST:/auth/login":
		return true
	default:
		return false
	}
}

func extractBearerToken(header string) (string, error) {
	value := strings.TrimSpace(header)
	if value == "" {
		return "", aop.UnauthorizedException("请在 Authorization 头中携带 Bearer Token")
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
		default:
			status = http.StatusInternalServerError
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

func currentAccount(ctx httpbinding.Context) (accountView, bool) {
	if ctx == nil {
		return accountView{}, false
	}
	value, ok := ctx.Get(authAccountKey)
	if !ok {
		return accountView{}, false
	}
	account, ok := value.(accountView)
	return account, ok
}

// ===== 模块定义：StoreModule =====

var storeToken = reflect.TypeFor[*memoryStore]()

// StoreModule 提供 *memoryStore 实例并导出，供其他模块通过 imports/exports 注入。
type StoreModule struct {
	*module.BaseModule
	store *memoryStore
}

func NewStoreModule() *StoreModule {
	store := newMemoryStore()
	pStore, _ := di.ProviderFromInstanceWithToken(storeToken, store, di.Singleton)
	base := module.NewBaseModule(&module.ModuleMetadata{
		Providers: []*di.Provider{pStore},
		Exports:   []any{storeToken},
	})
	return &StoreModule{BaseModule: base, store: store}
}

// 新 main：使用 imports/exports + 字段注入
//
// Controller 不再手动传 store，改为声明 Store 字段（exported），
// 由 BaseModule.Init() 中的 tryInjectOwner 自动从容器注入。
// StoreModule 通过 exports 传播 *memoryStore 到 ControllerModule，
// controller 作为零值传入，Init 时字段被自动填充。
func main() {
	// 1. 创建模块树
	storeModule := NewStoreModule()
	swaggerModule, err := swagger.ForRoot(
		swagger.Config{
			Title:       "lania-g HTTP Demo API",
			Description: "REST demo with Swagger docs",
			Version:     "1.0.0",
		},
		&swagger.UIConfig{
			Title:      "lania-g HTTP Demo Docs",
			SpecURL:    "/docs/openapi.json",
			SwaggerURL: "/docs",
			RedocURL:   "/redoc",
		},
	)
	if err != nil {
		panic(err)
	}

	// 2. Controller 传零值即可——字段注入会自动填充 Store
	//    tryInjectOwner 检测到所有字段为零值 → 走 Class construct 管线
	//    → reflect.New + injectFields → Store 字段从容器注入
	authCtrl := &AuthController{} // 零值，Init 时自动注入 Store
	userCtrl := &UserController{} // 零值，Init 时自动注入 Store

	root := module.CreateModule(
		[]module.Module{storeModule, swaggerModule},
		nil,
		[]any{authCtrl, userCtrl},
		nil, nil,
	)

	// 3. 创建应用
	httpAdapter := httpadapter.New().WithValidatorV10()
	reg := registry.New()
	app, err := application.NewWithOptions(root, application.Options{
		Registry:        reg,
		StartupReporter: os.Stdout,
	}, httpAdapter)
	if err != nil {
		panic(err)
	}

	// 4. DSL 声明（与改造前完全一致）
	httpAPI := httpAdapter.API().(*httpadapter.API)
	httpAPI.Controller("", authCtrl).
		Get("/", authCtrl.Home).
		Summary("Demo Home").
		Description("返回 demo 首页与可用路由列表。").
		Tags("system").
		ResponseEnvelope(apiEnvelope{}, "data").
		Build()

	httpAPI.Controller("/auth", authCtrl).
		Post("/register", authCtrl.Register).
		Summary("Register Account").
		Description("注册一个新账号。").
		Tags("auth").
		ResponseEnvelope(apiEnvelope{}, "data").
		ErrorResponse(http.StatusConflict, "账号已存在").
		StatusCode(http.StatusCreated).
		Post("/login", authCtrl.Login).
		Summary("Login").
		Description("使用账号密码登录并返回 Bearer Token。").
		Tags("auth").
		ResponseEnvelope(apiEnvelope{}, "data").
		ErrorResponse(http.StatusUnauthorized, "账号或密码错误").
		Build()

	httpAPI.Controller("/users", userCtrl).
		Get("", userCtrl.List).
		Summary("List Users").
		Description("分页查询用户列表，支持关键字过滤。").
		Tags("users").
		Security("bearerAuth").
		ResponseEnvelope(apiEnvelope{}, "data").
		ErrorResponse(http.StatusUnauthorized, "未登录或 token 无效").
		Get("/:id", userCtrl.Get).
		Summary("Get User").
		Description("根据用户 ID 获取用户详情。").
		Tags("users").
		Security("bearerAuth").
		ResponseEnvelope(apiEnvelope{}, "data").
		ErrorResponse(http.StatusUnauthorized, "未登录或 token 无效").
		ErrorResponse(http.StatusNotFound, "用户不存在").
		Post("", userCtrl.Create).
		Summary("Create User").
		Description("创建一个新的用户。").
		Tags("users").
		Security("bearerAuth").
		ResponseEnvelope(apiEnvelope{}, "data").
		ErrorResponse(http.StatusUnauthorized, "未登录或 token 无效").
		StatusCode(http.StatusCreated).
		Put("/:id", userCtrl.Update).
		Summary("Update User").
		Description("根据用户 ID 更新用户信息。").
		Tags("users").
		Security("bearerAuth").
		ResponseEnvelope(apiEnvelope{}, "data").
		ErrorResponse(http.StatusUnauthorized, "未登录或 token 无效").
		ErrorResponse(http.StatusNotFound, "用户不存在").
		Delete("/:id", userCtrl.Delete).
		Summary("Delete User").
		Description("根据用户 ID 删除用户。").
		Tags("users").
		Security("bearerAuth").
		ResponseEnvelope(apiEnvelope{}, "data").
		ErrorResponse(http.StatusUnauthorized, "未登录或 token 无效").
		ErrorResponse(http.StatusNotFound, "用户不存在").
		Build()

	// 5. 全局 interceptor
	//    从模块树获取 store 实例（authInterceptor 仍需要传 store）
	store, err := module.GetByType[*memoryStore](app.ModuleRef())
	if err != nil {
		panic(err)
	}
	app.UseGlobalInterceptors(responseEnvelopeInterceptor, authInterceptor(store))

	// 6. Swagger
	builder, err := module.GetByType[*swagger.Builder](app.ModuleRef())
	if err != nil {
		panic(err)
	}
	builder.AddBearerAuth("bearerAuth")
	builder.SetDefaultErrorResponse("Request failed", apiEnvelope{}, http.StatusBadRequest, http.StatusInternalServerError)
	if _, err := swagger.BuildFromHTTPRegistry(builder, reg); err != nil {
		panic(err)
	}
	swagger.ServeHTTPBridge(app)

	if _, err := app.CompileDiagnostics(); err != nil {
		panic(err)
	}
	if _, err := app.StartupReport(); err != nil {
		panic(err)
	}
	if err := app.Listen(":8080"); err != nil {
		panic(err)
	}

	select {}
}

// 旧 main — 手动构造模式，已由上方新 main 替代，保留供参考
//
// func oldMain() {
// 	store := newMemoryStore()
// 	authCtrl := &AuthController{Store: store}
// 	userCtrl := &UserController{Store: store}
// 	// ... 其余代码与原 main 相同
// }
