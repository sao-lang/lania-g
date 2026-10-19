// http-demo 演示 HTTP 协议的可运行组合：
//
//	模块树 + DI + 校验 + 五类 AOP（Middleware / Guard / Interceptor / Pipe / Filter）+ Swagger
//
// 运行：
//
//	cd cmd && go run ./http-demo
//
// 路由声明写在控制器的 Declare 方法里，因此自动归属于控制器所在的模块。
// 全局 AOP 通过 app.UseGlobal* 注册，路由/控制器级 AOP 通过 Routes 链声明。
package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

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

	roleAdmin  = "admin"
	roleMember = "member"
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
	Role     string
}

type accountView struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func (a account) view() accountView {
	return accountView{ID: a.ID, Username: a.Username, Role: a.Role}
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
	Role      string
}

// Store 是 demo 的内存存储，通过模块 provider 注入到控制器。
type Store struct {
	mu            sync.Mutex
	accounts      []account
	users         []user
	sessions      []session
	nextAccountID int
	nextUserID    int
}

func NewStore() *Store {
	return &Store{
		accounts: []account{
			{ID: 1, Username: "demo", Password: "demo1234", Role: roleAdmin},
			{ID: 2, Username: "guest", Password: "guest1234", Role: roleMember},
		},
		users: []user{
			{ID: 1, Name: "Alice", Email: "alice@example.com", Age: 20},
			{ID: 2, Name: "Bob", Email: "bob@example.com", Age: 26},
		},
		sessions:      make([]session, 0),
		nextAccountID: 3,
		nextUserID:    3,
	}
}

// Register 注册账号；用户名重复时返回 409。
func (s *Store) Register(username, password, role string) (accountView, error) {
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
		Role:     role,
	}
	s.nextAccountID++
	s.accounts = append(s.accounts, acc)
	return acc.view(), nil
}

// Login 校验账号密码并签发一个新的会话 token。
func (s *Store) Login(username, password string) (string, accountView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, item := range s.accounts {
		if strings.EqualFold(item.Username, username) && item.Password == password {
			token := fmt.Sprintf("token-%d-%d", item.ID, time.Now().UnixNano())
			s.sessions = append(s.sessions, session{
				Token:     token,
				AccountID: item.ID,
				Username:  item.Username,
				Role:      item.Role,
			})
			return token, item.view(), nil
		}
	}

	return "", accountView{}, aop.UnauthorizedException("账号或密码错误")
}

// AccountByToken 按 token 查账号。
func (s *Store) AccountByToken(token string) (account, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, item := range s.sessions {
		if item.Token == token {
			return account{ID: item.AccountID, Username: item.Username, Role: item.Role}, true
		}
	}
	return account{}, false
}

// ListUsers 按关键字过滤并分页。
func (s *Store) ListUsers(keyword string, page, size int) ([]user, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	keyword = strings.ToLower(strings.TrimSpace(keyword))
	filtered := make([]user, 0, len(s.users))
	for _, item := range s.users {
		if keyword == "" ||
			strings.Contains(strings.ToLower(item.Name), keyword) ||
			strings.Contains(strings.ToLower(item.Email), keyword) {
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

func (s *Store) CreateUser(req createUserRequest) user {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := user{ID: s.nextUserID, Name: req.Name, Email: req.Email, Age: req.Age}
	s.nextUserID++
	s.users = append(s.users, item)
	return item
}

func (s *Store) UpdateUser(id int, req updateUserRequest) (user, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.users {
		if s.users[i].ID == id {
			s.users[i].Name = req.Name
			s.users[i].Email = req.Email
			s.users[i].Age = req.Age
			return s.users[i], true
		}
	}
	return user{}, false
}

func (s *Store) DeleteUser(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.users {
		if s.users[i].ID == id {
			s.users = append(s.users[:i], s.users[i+1:]...)
			return true
		}
	}
	return false
}

func (s *Store) CountUsers() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.users)
}

func (s *Store) CountAccounts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.accounts)
}

// StoreModule 把 Store 作为 provider 导出给其他模块。
func StoreModule(store *Store) module.Module {
	token := reflect.TypeFor[*Store]()
	provider, _ := di.ProviderFromInstanceWithToken(token, store, di.Singleton)
	return module.New(module.Metadata{
		Providers: []*di.Provider{provider},
		Exports:   []any{token},
	})
}

// ---------------------------------------------------------------------------
// 请求 / 响应 DTO
// ---------------------------------------------------------------------------

type registerRequest struct {
	Username string `json:"username" validate:"required,min=3"`
	Password string `json:"password" validate:"required,min=6"`
	Role     string `json:"role" validate:"omitempty,oneof=admin member"`
}

type loginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type createUserRequest struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gte=0,lte=150"`
}

type updateUserRequest struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gte=0,lte=150"`
}

// ---------------------------------------------------------------------------
// AOP：Middleware
// ---------------------------------------------------------------------------

// loggingMiddleware 是最外层中间件：记录方法、路径、状态码与耗时。
//
// 注意：Guard / 参数绑定这类失败发生在中间件之后（由 ExceptionFilter 兜底），
// 中间件看不到最终状态码，因此这类请求记录成 ERR 而不是猜测一个状态码。
func loggingMiddleware(ctx *aop.ExecutionContext, next func() error) error {
	start := time.Now()
	err := next()
	elapsed := time.Since(start).Round(time.Millisecond)

	hc, _ := ctx.HandlerContext.(*runtime.HandlerContext)
	method, path := "", ""
	if hc != nil {
		method, path = hc.Request.Method, hc.Request.Path
	}

	if err != nil {
		log.Printf("[mw] %-6s %-18s ERR %v (%s)", method, path, err, elapsed)
		return err
	}

	status := http.StatusOK
	if hc != nil && hc.Response.Status > 0 {
		status = hc.Response.Status
	}
	log.Printf("[mw] %-6s %-18s %d %s", method, path, status, elapsed)
	return nil
}

// ---------------------------------------------------------------------------
// AOP：Pipe
// ---------------------------------------------------------------------------

// trimPipe 递归去掉结构体入参里字符串字段两端的空白，等价 NestJS 的全局 Pipe。
//
// 只处理结构体/字符串，遇到指针、接口、切片等原样返回，避免拷贝带锁对象。
func trimPipe(value interface{}, _ interface{}) (interface{}, error) {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text), nil
	}

	rv := reflect.ValueOf(value)
	if !rv.IsValid() || rv.Kind() != reflect.Struct {
		return value, nil
	}

	out := reflect.New(rv.Type()).Elem()
	out.Set(rv)
	trimStrings(out, 0)
	return out.Interface(), nil
}

func trimStrings(value reflect.Value, depth int) {
	if depth > 3 {
		return
	}
	typ := value.Type()
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		if !field.CanSet() {
			continue
		}
		switch field.Kind() {
		case reflect.String:
			field.SetString(strings.TrimSpace(field.String()))
		case reflect.Struct:
			if typ.Field(i).Type == reflect.TypeFor[time.Time]() {
				continue
			}
			trimStrings(field, depth+1)
		}
	}
}

// ---------------------------------------------------------------------------
// AOP：Interceptor
// ---------------------------------------------------------------------------

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

// authInterceptor 校验 Authorization 并把当前账号写入请求上下文。
//
// 公开路由（首页与登录/注册）直接放行；其余路由必须携带有效 token。
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

// timingInterceptor 是控制器级拦截器：给响应加一个耗时响应头。
func timingInterceptor(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
	start := time.Now()
	result, err := next.Handle()
	if hc, ok := ctx.HandlerContext.(*runtime.HandlerContext); ok {
		hc.Response.Headers["X-Elapsed-Ms"] = fmt.Sprintf("%d", time.Since(start).Milliseconds())
	}
	return result, err
}

// ---------------------------------------------------------------------------
// AOP：Guard
// ---------------------------------------------------------------------------

// adminGuard 是路由级守卫：只有 admin 角色能通过。
func adminGuard(store *Store) aop.GuardFunc {
	return func(ctx *aop.ExecutionContext) (bool, error) {
		hc, ok := ctx.HandlerContext.(*runtime.HandlerContext)
		if !ok || hc == nil {
			return false, aop.InternalServerErrorException("request context unavailable")
		}

		token, err := extractBearerToken(hc.Request.Headers["Authorization"])
		if err != nil {
			return false, err
		}
		acc, ok := store.AccountByToken(token)
		if !ok {
			return false, aop.UnauthorizedException("登录已失效，请重新登录")
		}
		if acc.Role != roleAdmin {
			return false, aop.ForbiddenException("需要管理员权限")
		}
		return true, nil
	}
}

// ---------------------------------------------------------------------------
// AOP：ExceptionFilter
// ---------------------------------------------------------------------------

// envelopeExceptionFilter 处理发生在拦截器之外的错误。
//
// 参数绑定与校验在进入 interceptor 链之前就会失败（等价于 NestJS 中
// ValidationPipe 抛出的异常），因此需要 ExceptionFilter 把它们也整形为统一 envelope。
func envelopeExceptionFilter(exception interface{}, ctx *aop.ExecutionContext) error {
	err, ok := exception.(error)
	if !ok || err == nil {
		return nil
	}

	hc, ok := ctx.HandlerContext.(*runtime.HandlerContext)
	if !ok || hc == nil {
		return err
	}
	httpCtx, ok := httpContextOf(hc)
	if !ok {
		return err
	}

	status, msg := statusAndMessageFromError(err)
	httpCtx.JSON(status, apiEnvelope{Msg: msg, Code: status, Data: nil, Success: false})
	return nil
}

// ---------------------------------------------------------------------------
// 辅助函数
// ---------------------------------------------------------------------------

// isPublicRoute 判断当前路由是否允许匿名访问。
func isPublicRoute(routeKey string) bool {
	switch routeKey {
	case "http:GET:/", "http:POST:/auth/login", "http:POST:/auth/register":
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

// httpContextOf 从请求上下文取出 HTTP 绑定上下文。
func httpContextOf(hc *runtime.HandlerContext) (*httpbinding.HttpContext, bool) {
	value, ok := hc.Get(httpbinding.MetadataKeyContext)
	if !ok {
		return nil, false
	}
	httpCtx, ok := value.(*httpbinding.HttpContext)
	if !ok || httpCtx == nil {
		return nil, false
	}
	return httpCtx, true
}

// currentAccount 读取 authInterceptor 写入的当前账号。
func currentAccount(hc *runtime.HandlerContext) (account, bool) {
	if hc == nil {
		return account{}, false
	}
	value, ok := hc.Get(sessionKey)
	if !ok {
		return account{}, false
	}
	acc, ok := value.(account)
	return acc, ok
}

// validateCredentials 演示 handler 内主动抛业务异常。
func validateCredentials(username, password string) error {
	if strings.TrimSpace(username) == "" {
		return aop.BadRequestException("username 不能为空")
	}
	if len(strings.TrimSpace(password)) < 6 {
		return aop.BadRequestException("password 长度不能少于 6 位")
	}
	return nil
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

// ---------------------------------------------------------------------------
// 控制器
// ---------------------------------------------------------------------------

// SystemController 演示公开的系统接口。
type SystemController struct {
	Store *Store
}

// Home 是公开的首页。
func (c *SystemController) Home() map[string]any {
	return map[string]any{
		"service": "lania-g http-demo",
		"routes": []string{
			"GET    /",
			"POST   /auth/register",
			"POST   /auth/login",
			"GET    /auth/me",
			"GET    /users?keyword=&page=&size=",
			"GET    /users/:id",
			"POST   /users",
			"PUT    /users/:id",
			"DELETE /users/:id",
			"GET    /admin/stats",
			"GET    /docs",
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

// AuthController 演示注册、登录与“当前登录用户”。
type AuthController struct {
	Store *Store
}

type registerArgs struct {
	Ctx  httpbinding.Context
	Body httpbinding.Body[registerRequest]
}

// Register 注册账号，成功返回 201，重复用户名返回 409。
func (c *AuthController) Register(args registerArgs) (any, error) {
	req := args.Body.Value
	if err := validateCredentials(req.Username, req.Password); err != nil {
		return nil, err
	}

	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = roleMember
	}

	account, err := c.Store.Register(req.Username, req.Password, role)
	if err != nil {
		return nil, err
	}

	// 保持 HTTP 语义：注册会创建资源，因此返回 201。
	args.Ctx.Status(http.StatusCreated)
	return map[string]any{"account": account}, nil
}

type loginArgs struct {
	Ctx  httpbinding.Context
	Body httpbinding.Body[loginRequest]
}

// Login 登录并返回 token 与账号信息。
func (c *AuthController) Login(args loginArgs) (any, error) {
	req := args.Body.Value
	if err := validateCredentials(req.Username, req.Password); err != nil {
		return nil, err
	}

	token, account, err := c.Store.Login(req.Username, req.Password)
	if err != nil {
		return nil, err
	}
	return map[string]any{"token": token, "account": account}, nil
}

type meArgs struct {
	Ctx httpbinding.Context
}

// Me 返回当前登录账号（由 authInterceptor 写入上下文）。
func (c *AuthController) Me(args meArgs) (any, error) {
	hc, ok := args.Ctx.(*httpbinding.HttpContext)
	if !ok || hc == nil {
		return nil, aop.InternalServerErrorException("request context unavailable")
	}
	acc, ok := currentAccountFromContext(hc)
	if !ok {
		return nil, aop.UnauthorizedException("未登录")
	}
	return acc.view(), nil
}

// Declare 声明认证路由。
func (c *AuthController) Declare(r *httpadapter.Routes) {
	r.Prefix("/auth").
		Post("/register", c.Register).
		Summary("Register Account").
		Description("注册一个新账号。").
		Tags("auth").
		Status(http.StatusCreated).
		ResponseEnvelope(apiEnvelope{}, "data").
		ErrorResponse(http.StatusConflict, "账号已存在").
		Post("/login", c.Login).
		Summary("Login").
		Description("使用账号密码登录并返回 token。").
		Tags("auth").
		ResponseEnvelope(apiEnvelope{}, "data").
		ErrorResponse(http.StatusUnauthorized, "账号或密码错误").
		Get("/me", c.Me).
		Summary("Current Account").
		Description("返回当前登录账号。").
		Tags("auth").
		Security("bearerAuth").
		ResponseEnvelope(apiEnvelope{}, "data")
}

// UserController 演示受保护接口（控制器级拦截器 + 全局过滤器）。
type UserController struct {
	Store *Store
}

type listUsersArgs struct {
	Ctx     httpbinding.Context
	Page    httpbinding.Query[int]     `query:"page"`
	Size    httpbinding.Query[int]     `query:"size"`
	Keyword httpbinding.Query[string]  `query:"keyword"`
	TraceID httpbinding.Header[string] `header:"X-Trace-Id"`
}

// List 返回分页用户列表。
func (c *UserController) List(args listUsersArgs) (any, error) {
	page := args.Page.Value
	size := args.Size.Value
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 10
	}

	items, total := c.Store.ListUsers(args.Keyword.Value, page, size)
	return map[string]any{
		"items":    items,
		"total":    total,
		"page":     page,
		"size":     size,
		"keyword":  args.Keyword.Value,
		"trace_id": args.TraceID.Value,
	}, nil
}

type getUserArgs struct {
	Ctx httpbinding.Context
	ID  httpbinding.Param[int] `param:"id" required:"true"`
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

// Create 创建用户，成功返回 201。
func (c *UserController) Create(args createUserArgs) (any, error) {
	item := c.Store.CreateUser(args.Body.Value)
	args.Ctx.Status(http.StatusCreated)
	return item, nil
}

type updateUserArgs struct {
	Ctx  httpbinding.Context
	ID   httpbinding.Param[int] `param:"id" required:"true"`
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
	ID  httpbinding.Param[int] `param:"id" required:"true"`
}

// Delete 删除用户。
func (c *UserController) Delete(args deleteUserArgs) (any, error) {
	if !c.Store.DeleteUser(args.ID.Value) {
		return nil, aop.NotFoundException("用户不存在")
	}
	return map[string]any{"deleted_id": args.ID.Value}, nil
}

// Declare 声明用户路由。
func (c *UserController) Declare(r *httpadapter.Routes) {
	// 前缀之后、第一条路由之前的 Use* 作用于整个控制器。
	r.Prefix("/users").UseInterceptors(timingInterceptor)

	r.Get("", c.List).
		Summary("List Users").
		Description("分页查询用户列表，支持关键字过滤。").
		Tags("users").
		Security("bearerAuth").
		ResponseEnvelope(apiEnvelope{}, "data").
		ErrorResponse(http.StatusUnauthorized, "未登录或 token 无效").
		Get("/:id", c.Get).
		Summary("Get User").
		Description("根据用户 ID 获取用户详情。").
		Tags("users").
		Security("bearerAuth").
		ResponseEnvelope(apiEnvelope{}, "data").
		ErrorResponse(http.StatusNotFound, "用户不存在").
		Post("", c.Create).
		Summary("Create User").
		Description("创建一个新的用户。").
		Tags("users").
		Security("bearerAuth").
		Status(http.StatusCreated).
		ResponseEnvelope(apiEnvelope{}, "data").
		ErrorResponse(http.StatusBadRequest, "参数错误").
		Put("/:id", c.Update).
		Summary("Update User").
		Description("根据用户 ID 更新用户信息。").
		Tags("users").
		Security("bearerAuth").
		ResponseEnvelope(apiEnvelope{}, "data").
		ErrorResponse(http.StatusNotFound, "用户不存在").
		Delete("/:id", c.Delete).
		Summary("Delete User").
		Description("根据用户 ID 删除用户。").
		Tags("users").
		Security("bearerAuth").
		ResponseEnvelope(apiEnvelope{}, "data").
		ErrorResponse(http.StatusNotFound, "用户不存在")
}

// AdminController 演示路由级守卫（非 admin 返回 403）。
type AdminController struct {
	Store *Store
}

// Stats 返回运行时统计。
func (c *AdminController) Stats() map[string]any {
	return map[string]any{
		"users":    c.Store.CountUsers(),
		"accounts": c.Store.CountAccounts(),
	}
}

// Declare 声明管理端路由。
func (c *AdminController) Declare(r *httpadapter.Routes) {
	r.Get("/admin/stats", c.Stats).
		UseGuards(adminGuard(c.Store)).
		Summary("Admin Stats").
		Description("仅管理员可访问。").
		Tags("admin").
		Security("bearerAuth").
		ResponseEnvelope(apiEnvelope{}, "data").
		ErrorResponse(http.StatusForbidden, "需要管理员权限")
}

// currentAccountFromContext 从 HttpContext 读取当前账号。
func currentAccountFromContext(hc *httpbinding.HttpContext) (account, bool) {
	value, ok := hc.Get(sessionKey)
	if !ok {
		return account{}, false
	}
	acc, ok := value.(account)
	return acc, ok
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

	// 控制器传零值即可：字段注入会从模块容器补齐 *Store。
	root := module.New(module.Metadata{
		Imports: []module.Module{
			httpModule,
			swaggerModule,
			StoreModule(store),
		},
		Controllers: []any{
			&SystemController{},
			&AuthController{},
			&UserController{},
			&AdminController{},
		},
	})

	app, err := application.New(root, application.Options{Logger: os.Stdout})
	if err != nil {
		log.Fatal(err)
	}

	// 全局 AOP。
	// Middleware 在最外层，Interceptor 顺序即执行顺序，Filter 兜住 binding/校验错误。
	app.UseGlobalMiddlewares(loggingMiddleware)
	app.UseGlobalPipes(trimPipe)
	app.UseGlobalInterceptors(responseEnvelopeInterceptor, authInterceptor(store))
	app.UseGlobalFilters(envelopeExceptionFilter)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
