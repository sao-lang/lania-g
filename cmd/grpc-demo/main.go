// grpc-demo 是 gRPC 集成的能力演示。
// 它不是推荐的业务项目模板，业务接入优先参考 `docs/标准接入模板.md`。
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/kernel/v3/aop"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	kerrors "github.com/sao-lang/lania-g/kernel/v3/errors"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	grpcadapter "github.com/sao-lang/lania-g/protocol/grpc/v3"
	grpcbinding "github.com/sao-lang/lania-g/protocol/grpc/v3/binding"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	authAccountKey = "demo.auth.account"
	authTokenKey   = "demo.auth.token"
)

// ===== 演示模型（结构与 http-demo 类似） =====

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

// ===== 统一响应包裹结构（用于 gRPC） =====

func envelopeOK(data any) *structpb.Struct {
	return mustStruct(map[string]any{
		"success": true,
		"code":    0,
		"msg":     "ok",
		"data":    data,
	})
}

func envelopeFail(code int, msg string) *structpb.Struct {
	return mustStruct(map[string]any{
		"success": false,
		"code":    code,
		"msg":     msg,
		"data":    nil,
	})
}

func mustStruct(m map[string]any) *structpb.Struct {
	s, err := structpb.NewStruct(m)
	if err != nil {
		// 演示代码里如果遇到意外的序列化错误，会直接提前 panic。
		panic(err)
	}
	return s
}

func errorCodeAndMessage(err error) (int, string) {
	if err == nil {
		return 0, "ok"
	}
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

// grpcEnvelopeInterceptor 让所有 unary 响应共享统一的 envelope 结构。
// 同时它会把错误折叠进 envelope 中，便于演示时保持传输层成功返回。
func grpcEnvelopeInterceptor(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
	out, err := next.Handle()
	if err != nil {
		code, msg := errorCodeAndMessage(err)
		return envelopeFail(code, msg), nil
	}
	if s, ok := out.(*structpb.Struct); ok {
		return s, nil
	}
	return envelopeOK(out), nil
}

func grpcAuthInterceptor(store *memoryStore) aop.InterceptorFunc {
	return func(ctx *aop.ExecutionContext, next aop.CallHandler) (any, error) {
		if isPublicGRPCRoute(ctx.RouteKey) {
			return next.Handle()
		}

		hc, _ := ctx.HandlerContext.(*runtime.HandlerContext)
		if hc == nil {
			return nil, aop.InternalServerErrorException("request context unavailable")
		}

		token, err := extractBearerToken(hc.Request.Headers["authorization"])
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

func isPublicGRPCRoute(routeKey string) bool {
	switch routeKey {
	case "grpc:Register:AuthService",
		"grpc:Login:AuthService",
		"grpc:WatchUsers:StreamService",
		"grpc:UploadUsers:StreamService",
		"grpc:ChatUsers:StreamService":
		return true
	default:
		return false
	}
}

func extractBearerToken(header string) (string, error) {
	value := strings.TrimSpace(header)
	if value == "" {
		return "", aop.UnauthorizedException("请在 Authorization 中携带 Bearer Token")
	}
	parts := strings.Fields(value)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") && strings.TrimSpace(parts[1]) != "" {
		return parts[1], nil
	}
	// 为了简化演示，这里直接接受原始 token。
	if strings.TrimSpace(value) != "" && !strings.Contains(value, " ") {
		return value, nil
	}
	return "", aop.UnauthorizedException("Authorization 格式应为 Bearer <token>")
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

func userPayload(item user) map[string]any {
	return map[string]any{
		"id":    item.ID,
		"name":  item.Name,
		"email": item.Email,
		"age":   item.Age,
	}
}

// ===== 服务实现 =====

type AuthService struct{}

type authPayloadArgs struct {
	Req *structpb.Struct `req:"true" required:"true"`
}

type meArgs struct {
	HC *runtime.HandlerContext
}

type getUserArgs struct {
	Req *structpb.Struct `req:"true" required:"true"`
}

type deleteUserArgs struct {
	Req *structpb.Struct `req:"true" required:"true"`
}

func (s *AuthService) Register(args authPayloadArgs, store *memoryStore) (any, error) {
	username := args.Req.GetFields()["username"].GetStringValue()
	password := args.Req.GetFields()["password"].GetStringValue()
	if err := validateCredentials(username, password); err != nil {
		return nil, err
	}
	acc, err := store.register(username, password)
	if err != nil {
		return nil, err
	}
	return map[string]any{"account": acc}, nil
}

func (s *AuthService) Login(args authPayloadArgs, store *memoryStore) (any, error) {
	username := args.Req.GetFields()["username"].GetStringValue()
	password := args.Req.GetFields()["password"].GetStringValue()
	if err := validateCredentials(username, password); err != nil {
		return nil, err
	}
	token, acc, err := store.login(username, password)
	if err != nil {
		return nil, err
	}
	return map[string]any{"token": token, "account": acc}, nil
}

type UserService struct{}

type listUsersArgs struct {
	// gRPC unary demo 现在默认推荐用 CompositeStruct 收口请求消息。
	Req *structpb.Struct `req:"true" required:"true"`
	// header tag 直接声明需要读取的 metadata key；BindParam(...) 只保留作兼容路径。
	TraceID string `header:"X-Trace-Id"`
}

type updateUserArgs struct {
	// Update 和其他 unary handler 一样，统一走 CompositeStruct 主路径。
	Req *structpb.Struct `req:"true" required:"true"`
}

type createUserDTO struct {
	Name  string `json:"name" validate:"required,min=2"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gt=0"`
}

func (s *UserService) Me(args meArgs) (any, error) {
	acc, ok := currentAccount(args.HC)
	if !ok {
		return nil, aop.UnauthorizedException("未登录")
	}
	return map[string]any{"account": acc}, nil
}

func (s *UserService) List(args listUsersArgs, store *memoryStore) (any, error) {
	keyword := args.Req.GetFields()["keyword"].GetStringValue()
	page := int(args.Req.GetFields()["page"].GetNumberValue())
	size := int(args.Req.GetFields()["size"].GetNumberValue())
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 10
	}
	items, total := store.listUsers(keyword, page, size)
	return map[string]any{
		"list":     items,
		"total":    total,
		"page":     page,
		"size":     size,
		"keyword":  keyword,
		"trace_id": args.TraceID,
	}, nil
}

func (s *UserService) Get(args getUserArgs, store *memoryStore) (any, error) {
	id := int(args.Req.GetFields()["id"].GetNumberValue())
	if id <= 0 {
		return nil, aop.BadRequestException("id 必须是正整数")
	}
	u, err := store.getUserByID(id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"user": u}, nil
}

func (s *UserService) Create(ctx grpcbinding.GRPCContext, store *memoryStore) (any, error) {
	var dto createUserDTO
	if err := ctx.ShouldBindReq(&dto); err != nil {
		return nil, err
	}
	return store.createUser(dto.Name, dto.Email, dto.Age), nil
}

func (s *UserService) Update(args updateUserArgs, store *memoryStore) (any, error) {
	id := int(args.Req.GetFields()["id"].GetNumberValue())
	name := args.Req.GetFields()["name"].GetStringValue()
	email := args.Req.GetFields()["email"].GetStringValue()
	age := int(args.Req.GetFields()["age"].GetNumberValue())
	if id <= 0 {
		return nil, aop.BadRequestException("id 必须是正整数")
	}
	if strings.TrimSpace(name) == "" {
		return nil, aop.BadRequestException("name 不能为空")
	}
	if strings.TrimSpace(email) == "" {
		return nil, aop.BadRequestException("email 不能为空")
	}
	if age <= 0 {
		return nil, aop.BadRequestException("age 必须大于 0")
	}
	u, err := store.updateUser(id, name, email, age)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *UserService) Delete(args deleteUserArgs, store *memoryStore) (any, error) {
	id := int(args.Req.GetFields()["id"].GetNumberValue())
	if id <= 0 {
		return nil, aop.BadRequestException("id 必须是正整数")
	}
	if err := store.deleteUser(id); err != nil {
		return nil, err
	}
	return map[string]any{"deleted_id": id}, nil
}

type StreamService struct{}

type watchUsersArgs struct {
	// streaming demo 现在也统一采用 CompositeStruct，把首请求和发送流收口到一个 DTO 里。
	Req *structpb.Struct `req:"true" required:"true"`
	Ctx grpcbinding.GRPCContext
	// server-stream 场景下，业务通过该字段显式控制连续 Send 的节奏。
	Stream grpcbinding.ServerStream[*structpb.Struct]
}

type uploadUsersArgs struct {
	// client-stream 通过聚合参数暴露 Recv 能力，避免签名重新回到“多个顶层参数”的旧风格。
	Stream grpcbinding.ClientStream[*structpb.Struct]
	Ctx    grpcbinding.GRPCContext
	// 保留 Raw 示例，方便需要更底层 gRPC 能力时直接逃生。
	Raw grpcbinding.RawServerStream
}

type chatUsersArgs struct {
	// bidi-stream 可以直接放进 CompositeStruct，读写仍然由 handler 显式驱动。
	Stream grpcbinding.BidiStream[*structpb.Struct, *structpb.Struct]
	Ctx    grpcbinding.GRPCContext
}

type uploadUserDTO struct {
	Name  string `json:"name" validate:"required,min=2"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"gt=0"`
}

type chatUserDTO struct {
	Text string `json:"text" validate:"required"`
}

type watchUsersDTO struct {
	Keyword string `json:"keyword"`
	Count   int    `json:"count" validate:"gte=0"`
}

func (s *StreamService) WatchUsers(args watchUsersArgs, store *memoryStore) error {
	// 这个示例演示 server streaming 的典型形态：
	// 先接收一次查询条件，然后连续向客户端推送多条事件/数据。
	var dto watchUsersDTO
	if err := args.Ctx.ShouldBindReq(&dto); err != nil {
		return err
	}
	keyword := dto.Keyword
	count := dto.Count
	if count <= 0 {
		count = 3
	}
	items, _ := store.listUsers(keyword, 1, count)
	for i, item := range items {
		if err := args.Stream.Send(mustStruct(map[string]any{
			"seq":     i + 1,
			"keyword": keyword,
			"user":    userPayload(item),
		})); err != nil {
			return err
		}
	}
	return nil
}

func (s *StreamService) UploadUsers(args uploadUsersArgs, store *memoryStore) (any, error) {
	// 这个示例演示 client streaming：
	// handler 主动循环读取客户端上传的多条消息，并在 EOF 后返回一次聚合结果。
	if args.Raw.ServerStream == nil {
		return nil, aop.InternalServerErrorException("grpc raw stream 未注入")
	}
	received := 0
	lastUser := map[string]any(nil)
	for {
		chunk, err := args.Stream.Recv()
		if err == io.EOF {
			return map[string]any{
				"received":  received,
				"last_user": lastUser,
			}, nil
		}
		if err != nil {
			return nil, err
		}
		var dto uploadUserDTO
		if err := args.Ctx.ShouldBindStream(chunk, &dto); err != nil {
			return nil, err
		}
		created := store.createUser(dto.Name, dto.Email, dto.Age)
		received++
		lastUser = userPayload(created)
	}
}

func (s *StreamService) ChatUsers(args chatUsersArgs) error {
	// 这个示例演示 bidi streaming：
	// 每收到一条消息就立即回一条响应，读写节奏完全由 handler 自己控制。
	for {
		msg, err := args.Stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		var dto chatUserDTO
		if err := args.Ctx.ShouldBindStream(msg, &dto); err != nil {
			return err
		}
		text := strings.TrimSpace(dto.Text)
		if err := args.Stream.Send(mustStruct(map[string]any{
			"text": text,
			"ack":  true,
			"echo": strings.ToUpper(text),
		})); err != nil {
			return err
		}
	}
}

func main() {
	store := newMemoryStore()
	authSvc := &AuthService{}
	userSvc := &UserService{}
	streamSvc := &StreamService{}

	// 这个 demo 故意把 receiver 放在显式 provider token 上，
	// 这样能力示例也能同时展示 DI token 连接和 value provider 的归属关系。
	// 新业务代码不必只依赖这一种模式。
	pStore, err := di.ProviderFromInstanceWithToken(reflect.TypeFor[*memoryStore](), store, di.Singleton)
	if err != nil {
		panic(err)
	}
	pAuth, err := di.ProviderFromInstanceWithToken(reflect.TypeFor[*AuthService](), authSvc, di.Singleton)
	if err != nil {
		panic(err)
	}
	pUser, err := di.ProviderFromInstanceWithToken(reflect.TypeFor[*UserService](), userSvc, di.Singleton)
	if err != nil {
		panic(err)
	}
	pStream, err := di.ProviderFromInstanceWithToken(reflect.TypeFor[*StreamService](), streamSvc, di.Singleton)
	if err != nil {
		panic(err)
	}
	root := module.CreateModule(nil, []*di.Provider{pStore, pAuth, pUser, pStream}, nil, nil, nil)

	// 启用 server reflection，这样即使本地没有 `.proto` 也能用 `grpcurl` 调试。
	server := grpc.NewServer()
	reflection.Register(server)

	grpcAdp := grpcadapter.New(":50051").WithServer(server)
	app, err := application.NewWithOptions(root, application.Options{
		Registry:        registry.New(),
		StartupReporter: os.Stdout,
	}, grpcAdp)
	if err != nil {
		panic(err)
	}

	grpcAPI := grpcAdp.API().(*grpcadapter.API)
	grpcAPI.Service("AuthService", authSvc).
		Method("Register", authSvc.Register).
		Method("Login", authSvc.Login).
		Build()

	grpcAPI.Service("UserService", userSvc).
		Method("Me", userSvc.Me).
		Method("List", userSvc.List). // demonstrate grpc composite + tag binding
		Method("Get", userSvc.Get).
		Method("Create", userSvc.Create).WithReqType((*structpb.Struct)(nil)).
		Method("Update", userSvc.Update).
		Method("Delete", userSvc.Delete).
		Build()

	grpcAPI.Service("StreamService", streamSvc).
		ServerStreamMethod("WatchUsers", streamSvc.WatchUsers).
		ClientStreamMethod("UploadUsers", streamSvc.UploadUsers).
		BidiStreamMethod("ChatUsers", streamSvc.ChatUsers).
		Build()

	// 这里保留全局 interceptor，用来在一个文件里同时演示鉴权与响应整形；
	// 它会刻意比业务 app demo 更偏高级能力展示。
	app.UseGlobalInterceptors(grpcEnvelopeInterceptor, grpcAuthInterceptor(store))

	if _, err := app.CompileDiagnostics(); err != nil {
		panic(err)
	}
	if _, err := app.StartupReport(); err != nil {
		panic(err)
	}
	if err := app.Run(); err != nil {
		panic(err)
	}

	select {}
}
