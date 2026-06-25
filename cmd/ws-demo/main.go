// ws-demo 是 WebSocket 集成的能力演示。
// 它不是推荐的业务项目模板，业务接入优先参考 `docs/标准接入模板.md`。
package main

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	httpadapter "github.com/sao-lang/lania-g/protocol/http/v3"
	wsadapter "github.com/sao-lang/lania-g/protocol/ws/v3"
	"github.com/sao-lang/lania-g/application/v3"
	wsbinding "github.com/sao-lang/lania-g/protocol/ws/v3/binding"
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
			token := "token-" + time.Now().Format("150405.000000000")
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
	status := 500
	msg := "服务器内部错误"

	var kerr *kerrors.KernelError
	if errors.As(err, &kerr) && kerr != nil {
		switch kerr.Kind {
		case kerrors.KindRouteNotFound:
			status = 404
		case kerrors.KindForbidden:
			status = 403
		case kerrors.KindUnauthorized:
			status = 401
		case kerrors.KindBinding, kerrors.KindValidation, kerrors.KindDI:
			status = 400
		default:
			status = 500
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

func wsEnvelopeInterceptor(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
	result, err := next.Handle()
	if err != nil {
		code, msg := statusAndMessageFromError(err)
		return apiEnvelope{Msg: msg, Code: code, Data: nil, Success: false}, nil
	}
	if env, ok := result.(apiEnvelope); ok {
		return env, nil
	}
	return apiEnvelope{Msg: "ok", Code: 0, Data: result, Success: true}, nil
}

func wsAuthInterceptor(store *memoryStore) aop.InterceptorFunc {
	return func(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
		if isPublicWSRoute(ctx.RouteKey) {
			return next.Handle()
		}
		hc, _ := ctx.HandlerContext.(*runtime.HandlerContext)
		if hc == nil || hc.Request == nil {
			return nil, aop.InternalServerErrorException("request context unavailable")
		}
		authHeader := hc.Request.Headers["Authorization"]
		if authHeader == "" {
			authHeader = hc.Request.Headers["authorization"]
		}
		token, err := extractBearerToken(authHeader)
		if err != nil {
			return nil, err
		}
		acc, ok := store.findAccountByToken(token)
		if !ok {
			return nil, aop.UnauthorizedException("登录已失效，请重新登录")
		}
		hc.Set(authAccountKey, acc)
		hc.Set(authTokenKey, token)
		return next.Handle()
	}
}

func isPublicWSRoute(routeKey string) bool {
	switch routeKey {
	case "ws:auth_register:/ws/chat", "ws:auth_login:/ws/chat":
		return true
	default:
		return false
	}
}

func currentAccount(ctx wsbinding.Context) (accountView, bool) {
	if ctx == nil || ctx.HandlerContext() == nil {
		return accountView{}, false
	}
	v, ok := ctx.HandlerContext().Get(authAccountKey)
	if !ok {
		return accountView{}, false
	}
	acc, ok := v.(accountView)
	return acc, ok
}

// ===== WebSocket 网关 =====

type ChatGateway struct {
}

func (g *ChatGateway) OnWebSocketConnect(conn any) error {
	_ = conn
	return nil
}

func (g *ChatGateway) OnWebSocketDisconnect(conn any, reason string) {
	_, _ = conn, reason
}

func (g *ChatGateway) OnWebSocketError(conn any, err error) {
	_, _ = conn, err
}

type registerDTO struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required,min=6"`
}

func (g *ChatGateway) AuthRegister(ctx wsbinding.Context, store *memoryStore) (any, error) {
	var dto registerDTO
	if err := ctx.ShouldBindMessage(&dto); err != nil {
		return nil, err
	}
	acc, err := store.register(dto.Username, dto.Password)
	if err != nil {
		return nil, err
	}
	return map[string]any{"account": acc}, nil
}

func (g *ChatGateway) AuthLogin(ctx wsbinding.Context, store *memoryStore) (any, error) {
	var dto registerDTO
	if err := ctx.ShouldBindMessage(&dto); err != nil {
		return nil, err
	}
	token, acc, err := store.login(dto.Username, dto.Password)
	if err != nil {
		return nil, err
	}
	return map[string]any{"token": token, "account": acc}, nil
}

type listUsersDTO struct {
	Page    int    `json:"page"`
	Size    int    `json:"size"`
	Keyword string `json:"keyword"`
}

func (g *ChatGateway) UsersList(ctx wsbinding.Context, body wsbinding.WSMessageBody[listUsersDTO], store *memoryStore) (any, error) {
	page := body.Value.Page
	size := body.Value.Size
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 10
	}
	items, total := store.listUsers(body.Value.Keyword, page, size)
	acc, _ := currentAccount(ctx)
	return map[string]any{
		"list":    items,
		"total":   total,
		"page":    page,
		"size":    size,
		"keyword": body.Value.Keyword,
		"account": acc,
	}, nil
}

type idDTO struct {
	ID int `json:"id"`
}

func (g *ChatGateway) UsersGet(body wsbinding.WSMessageBody[idDTO], store *memoryStore) (any, error) {
	if body.Value.ID <= 0 {
		return nil, aop.BadRequestException("id 必须是正整数")
	}
	u, err := store.getUserByID(body.Value.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"user": u}, nil
}

type createUserDTO struct {
	Name  string `json:"name" validate:"required,min=1"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gt=0"`
}

func (g *ChatGateway) UsersCreate(ctx wsbinding.Context, store *memoryStore) (any, error) {
	var dto createUserDTO
	if err := ctx.ShouldBindMessage(&dto); err != nil {
		return nil, err
	}
	return store.createUser(dto.Name, dto.Email, dto.Age), nil
}

type updateUserDTO struct {
	ID    int    `json:"id" validate:"gt=0"`
	Name  string `json:"name" validate:"required,min=1"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gt=0"`
}

func (g *ChatGateway) UsersUpdate(ctx wsbinding.Context, store *memoryStore) (any, error) {
	var dto updateUserDTO
	if err := ctx.ShouldBindMessage(&dto); err != nil {
		return nil, err
	}
	u, err := store.updateUser(dto.ID, dto.Name, dto.Email, dto.Age)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (g *ChatGateway) UsersDelete(body wsbinding.WSMessageBody[idDTO], store *memoryStore) (any, error) {
	if body.Value.ID <= 0 {
		return nil, aop.BadRequestException("id 必须是正整数")
	}
	if err := store.deleteUser(body.Value.ID); err != nil {
		return nil, err
	}
	return map[string]any{"deleted_id": body.Value.ID}, nil
}

type roomJoinDTO struct {
	Room string `json:"room" validate:"required"`
}

func (g *ChatGateway) RoomJoin(ctx wsbinding.Context) (any, error) {
	var dto roomJoinDTO
	if err := ctx.ShouldBindMessage(&dto); err != nil {
		return nil, err
	}
	_ = ctx.Join(dto.Room)
	return map[string]any{"id": ctx.ID(), "rooms": ctx.Rooms()}, nil
}

type roomBroadcastDTO struct {
	Room string `json:"room" validate:"required"`
	Msg  string `json:"msg" validate:"required,min=1"`
}

func (g *ChatGateway) RoomBroadcast(ctx wsbinding.Context) (any, error) {
	var dto roomBroadcastDTO
	if err := ctx.ShouldBindMessage(&dto); err != nil {
		return nil, err
	}
	_ = ctx.BroadcastTo(dto.Room, "room_message", map[string]any{
		"from": ctx.ID(),
		"msg":  dto.Msg,
	})
	return map[string]any{"sent": true}, nil
}

func main() {
	store := newMemoryStore()
	gw := &ChatGateway{}

	// 这个 demo 故意保留 provider 连接与共享监听集成，
	// 以便同时展示 gateway 归属、DI，以及挂载到 HTTP 上的 socket.io 行为。
	pStore, err := di.ProviderFromInstanceWithToken(reflect.TypeFor[*memoryStore](), store, di.Singleton)
	if err != nil {
		panic(err)
	}

	// WS gateway 需要落到模块 receiver 槽位，这里使用 controllers 槽位承载它。
	root := module.CreateModule(nil, []*di.Provider{pStore}, []interface{}{gw}, nil, nil)

	httpAdp := httpadapter.New()
	wsAdp := wsadapter.New() // shared listen: mount to http adapter at /socket.io/

	app, err := application.NewWithOptions(root, application.Options{
		Registry:        registry.New(),
		StartupReporter: os.Stdout,
	}, httpAdp, wsAdp)
	if err != nil {
		panic(err)
	}

	wsAPI := wsAdp.API().(*wsadapter.API)
	wsAPI.Gateway("/ws/chat", gw).
		On("auth_register", gw.AuthRegister).
		On("auth_login", gw.AuthLogin).
		On("users_list", gw.UsersList).
		On("users_get", gw.UsersGet).
		On("users_create", gw.UsersCreate).
		On("users_update", gw.UsersUpdate).
		On("users_delete", gw.UsersDelete).
		On("room_join", gw.RoomJoin).
		On("room_broadcast", gw.RoomBroadcast).
		Build()

	// 这里使用全局 interceptor 来演示跨 socket event 的 envelope / 鉴权行为；
	// `cmd/ws-app-demo` 则是更精简、更偏业务起步的版本。
	app.UseGlobalInterceptors(wsEnvelopeInterceptor, wsAuthInterceptor(store))

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
