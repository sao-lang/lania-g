// graphql-demo 是 GraphQL 集成的能力演示。
// 它不是推荐的业务项目模板，业务接入优先参考 `docs/标准接入模板.md`。
package main

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	graphqladapter "github.com/sao-lang/lania-g/protocol/graphql/v3"
	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
	"github.com/sao-lang/lania-g/application/v3"
	gqlbinding "github.com/sao-lang/lania-g/protocol/graphql/v3/binding"
	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

const (
	authAccountKey = "demo.auth.account"
	authTokenKey   = "demo.auth.token"
)

type BaseResponse struct {
	Success bool   `json:"success"`
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
}

func okBase() BaseResponse { return BaseResponse{Success: true, Code: 0, Msg: "ok"} }
func errBase(code int, msg string) BaseResponse {
	return BaseResponse{Success: false, Code: code, Msg: msg}
}

type memoryStore struct {
	mu            sync.Mutex
	accounts      []account
	users         []user
	sessions      []session
	nextAccountID int
	nextUserID    int
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
	acc := account{ID: s.nextAccountID, Username: username, Password: password}
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
			s.sessions = append(s.sessions, session{Token: token, AccountID: item.ID, Username: item.Username})
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
	item := user{ID: s.nextUserID, Name: name, Email: email, Age: age}
	s.nextUserID++
	s.users = append(s.users, item)
	return item
}

func (s *memoryStore) listUsers(keyword string, page, size int) ([]user, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	filtered := make([]user, 0, len(s.users))
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

func validateCredentials(username, password string) error {
	if strings.TrimSpace(username) == "" {
		return aop.BadRequestException("username 不能为空")
	}
	if len(strings.TrimSpace(password)) < 6 {
		return aop.BadRequestException("password 长度不能少于 6 位")
	}
	return nil
}

func statusAndMessageFromError(err error) (int, string) {
	code := 500
	msg := "服务器内部错误"

	var kerr *kerrors.KernelError
	if errors.As(err, &kerr) && kerr != nil {
		switch kerr.Kind {
		case kerrors.KindRouteNotFound:
			code = 404
		case kerrors.KindForbidden:
			code = 403
		case kerrors.KindUnauthorized:
			code = 401
		case kerrors.KindBinding, kerrors.KindValidation, kerrors.KindDI:
			code = 400
		default:
			code = 500
		}
		if strings.TrimSpace(kerr.Error()) != "" {
			msg = kerr.Error()
		}
		return code, msg
	}

	var httpErr *aop.HttpException
	if errors.As(err, &httpErr) && httpErr != nil {
		if httpErr.Status > 0 {
			code = httpErr.Status
		}
		if strings.TrimSpace(httpErr.Message) != "" {
			msg = httpErr.Message
		}
		return code, msg
	}

	if strings.TrimSpace(err.Error()) != "" {
		msg = err.Error()
	}
	return code, msg
}

func graphqlAuthGuard(store *memoryStore) aop.GuardFunc {
	return func(ctx *aop.ExecutionContext) (bool, error) {
		if isPublicGraphQLRoute(ctx.RouteKey) {
			return true, nil
		}
		hc, _ := ctx.HandlerContext.(*runtime.HandlerContext)
		if hc == nil || hc.Request == nil {
			return false, aop.InternalServerErrorException("request context unavailable")
		}
		authHeader := hc.Request.Headers["Authorization"]
		if authHeader == "" {
			authHeader = hc.Request.Headers["authorization"]
		}
		token, err := extractBearerToken(authHeader)
		if err != nil {
			return false, err
		}
		acc, ok := store.findAccountByToken(token)
		if !ok {
			return false, aop.UnauthorizedException("登录已失效，请重新登录")
		}
		hc.Set(authAccountKey, acc)
		hc.Set(authTokenKey, token)
		return true, nil
	}
}

func isPublicGraphQLRoute(routeKey string) bool {
	switch routeKey {
	case "graphql:Mutation:register", "graphql:Mutation:login":
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

func currentAccount(hc *runtime.HandlerContext) (accountView, bool) {
	if hc == nil {
		return accountView{}, false
	}
	v, ok := hc.Get(authAccountKey)
	if !ok {
		return accountView{}, false
	}
	acc, ok := v.(accountView)
	return acc, ok
}

// ===== GraphQL 返回结果类型（保持各操作返回结构一致） =====

type RegisterResult struct {
	BaseResponse
	Account accountView `json:"account"`
}

type LoginResult struct {
	BaseResponse
	Token   string      `json:"token"`
	Account accountView `json:"account"`
}

type MeResult struct {
	BaseResponse
	Account accountView `json:"account"`
}

type UserResult struct {
	BaseResponse
	User user `json:"user"`
}

type UserListResult struct {
	BaseResponse
	List    []user `json:"list"`
	Total   int    `json:"total"`
	Page    int    `json:"page"`
	Size    int    `json:"size"`
	Keyword string `json:"keyword"`
	TraceID string `json:"trace_id"`
	Account accountView
}

type DeleteResult struct {
	BaseResponse
	DeletedID int `json:"deleted_id"`
}

// ===== Resolver 实现 =====

type MutationResolver struct {
}

type QueryResolver struct {
}

type UserResolver struct{}

func (r *MutationResolver) Register(args struct {
	Username string `json:"username"`
	Password string `json:"password"`
}, store *memoryStore) (RegisterResult, error) {
	if err := validateCredentials(args.Username, args.Password); err != nil {
		code, msg := statusAndMessageFromError(err)
		return RegisterResult{BaseResponse: errBase(code, msg)}, nil
	}
	acc, err := store.register(args.Username, args.Password)
	if err != nil {
		code, msg := statusAndMessageFromError(err)
		return RegisterResult{BaseResponse: errBase(code, msg)}, nil
	}
	return RegisterResult{BaseResponse: okBase(), Account: acc}, nil
}

func (r *MutationResolver) Login(args struct {
	Username string `json:"username"`
	Password string `json:"password"`
}, store *memoryStore) (LoginResult, error) {
	if err := validateCredentials(args.Username, args.Password); err != nil {
		code, msg := statusAndMessageFromError(err)
		return LoginResult{BaseResponse: errBase(code, msg)}, nil
	}
	token, acc, err := store.login(args.Username, args.Password)
	if err != nil {
		code, msg := statusAndMessageFromError(err)
		return LoginResult{BaseResponse: errBase(code, msg)}, nil
	}
	return LoginResult{BaseResponse: okBase(), Token: token, Account: acc}, nil
}

func (r *MutationResolver) CreateUser(args struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}, store *memoryStore) (UserResult, error) {
	if strings.TrimSpace(args.Name) == "" {
		return UserResult{BaseResponse: errBase(400, "name 不能为空")}, nil
	}
	if strings.TrimSpace(args.Email) == "" {
		return UserResult{BaseResponse: errBase(400, "email 不能为空")}, nil
	}
	if args.Age <= 0 {
		return UserResult{BaseResponse: errBase(400, "age 必须大于 0")}, nil
	}
	u := store.createUser(args.Name, args.Email, args.Age)
	return UserResult{BaseResponse: okBase(), User: u}, nil
}

func (r *MutationResolver) UpdateUser(args struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}, store *memoryStore) (UserResult, error) {
	if args.ID <= 0 {
		return UserResult{BaseResponse: errBase(400, "id 必须是正整数")}, nil
	}
	if strings.TrimSpace(args.Name) == "" {
		return UserResult{BaseResponse: errBase(400, "name 不能为空")}, nil
	}
	if strings.TrimSpace(args.Email) == "" {
		return UserResult{BaseResponse: errBase(400, "email 不能为空")}, nil
	}
	if args.Age <= 0 {
		return UserResult{BaseResponse: errBase(400, "age 必须大于 0")}, nil
	}
	u, err := store.updateUser(args.ID, args.Name, args.Email, args.Age)
	if err != nil {
		code, msg := statusAndMessageFromError(err)
		return UserResult{BaseResponse: errBase(code, msg)}, nil
	}
	return UserResult{BaseResponse: okBase(), User: u}, nil
}

func (r *MutationResolver) DeleteUser(args struct {
	ID int `json:"id"`
}, store *memoryStore) (DeleteResult, error) {
	if args.ID <= 0 {
		return DeleteResult{BaseResponse: errBase(400, "id 必须是正整数")}, nil
	}
	if err := store.deleteUser(args.ID); err != nil {
		code, msg := statusAndMessageFromError(err)
		return DeleteResult{BaseResponse: errBase(code, msg)}, nil
	}
	return DeleteResult{BaseResponse: okBase(), DeletedID: args.ID}, nil
}

func (r *QueryResolver) Me(hc *runtime.HandlerContext) (MeResult, error) {
	acc, ok := currentAccount(hc)
	if !ok {
		return MeResult{BaseResponse: errBase(401, "未登录")}, nil
	}
	return MeResult{BaseResponse: okBase(), Account: acc}, nil
}

func (r *QueryResolver) UserWithStore(id gqlbinding.Arg[int], store *memoryStore) (UserResult, error) {
	if id.Value <= 0 {
		return UserResult{BaseResponse: errBase(400, "id 必须是正整数")}, nil
	}
	u, err := store.getUserByID(id.Value)
	if err != nil {
		code, msg := statusAndMessageFromError(err)
		return UserResult{BaseResponse: errBase(code, msg)}, nil
	}
	return UserResult{BaseResponse: okBase(), User: u}, nil
}

func (r *QueryResolver) Users(args struct {
	Page    int    `json:"page"`
	Size    int    `json:"size"`
	Keyword string `json:"keyword"`
}, trace gqlbinding.Header[string], hc *runtime.HandlerContext, store *memoryStore) (UserListResult, error) {
	page := args.Page
	size := args.Size
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 10
	}
	items, total := store.listUsers(args.Keyword, page, size)
	acc, _ := currentAccount(hc)
	return UserListResult{
		BaseResponse: okBase(),
		List:         items,
		Total:        total,
		Page:         page,
		Size:         size,
		Keyword:      args.Keyword,
		TraceID:      trace.Value,
		Account:      acc,
	}, nil
}

func (r *UserResolver) DisplayName(parent gqlbinding.Parent[user], field gqlbinding.FieldName, set gqlbinding.SelectionSet) (string, error) {
	_ = set
	if field != "displayName" {
		return "", nil
	}
	return parent.Value.Name + " (display)", nil
}

func main() {
	store := newMemoryStore()
	mutation := &MutationResolver{}
	query := &QueryResolver{}
	user := &UserResolver{}

	// 这个 demo 故意同时使用 provider 连接和多种 resolver 类型，
	// 以便一起观察 owner 解析、DI 与 schema 构建行为。
	pStore, err := di.ProviderFromInstanceWithToken(reflect.TypeFor[*memoryStore](), store, di.Singleton)
	if err != nil {
		panic(err)
	}
	// GraphQL resolver 需要落到模块 receiver 槽位，这里使用 resolvers 槽位承载它。
	root := module.CreateModule(nil, []*di.Provider{pStore}, nil, []interface{}{mutation, query, user}, nil)

	httpAdp := httpadapter.New()
	gqlAdp := graphqladapter.New().WithPlayground(true)

	app, err := application.NewWithOptions(root, application.Options{
		Registry:        registry.New(),
		StartupReporter: os.Stdout,
	}, httpAdp, gqlAdp)
	if err != nil {
		panic(err)
	}

	// 全局 GraphQL 配置演示：
	gqlAPI := gqlAdp.API().(*graphqladapter.API)
	gqlAPI.UseComplexityLimit(200)

	// 下面的 Query / Mutation / Object 定义刻意比 app demo 更丰富，
	// 这样可以把 guards、限流、cache-control 和 schema 元数据一起展示出来。
	gqlAPI.Resolver("Mutation", mutation).
		Mutation("register", mutation.Register).Arg("username").Arg("password").Returns("RegisterResult").Description("注册账号（公开接口）").
		Mutation("login", mutation.Login).Arg("username").Arg("password").Returns("LoginResult").Description("登录并返回 token（公开接口）").
		Mutation("createUser", mutation.CreateUser).Arg("name").Arg("email").Arg("age").Returns("UserResult").UseGuards(graphqlAuthGuard(store)).RequirePermission("user:create").
		Mutation("updateUser", mutation.UpdateUser).Arg("id").Arg("name").Arg("email").Arg("age").Returns("UserResult").UseGuards(graphqlAuthGuard(store)).Timeout(5000).
		Mutation("deleteUser", mutation.DeleteUser).Arg("id").Returns("DeleteResult").UseGuards(graphqlAuthGuard(store)).Deprecation("演示字段：生产环境建议软删除").
		Build()

	gqlAPI.Resolver("Query", query).
		Query("me", query.Me).Returns("MeResult").UseGuards(graphqlAuthGuard(store)).CacheControl("private, max-age=0").
		Query("user", query.UserWithStore).Arg("id").Returns("UserResult").UseGuards(graphqlAuthGuard(store)).
		Query("users", query.Users).Arg("page").Arg("size").Arg("keyword").Returns("UserListResult").UseGuards(graphqlAuthGuard(store)).RateLimit(20, 1000).
		Build()

	gqlAPI.Resolver("User", user).
		Object("displayName", user.DisplayName).
		Build()

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
