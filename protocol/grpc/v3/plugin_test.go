package grpc

import (
	stdctx "context"
	"io"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/grpc"
	grpcmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
	grpcbinding "github.com/sao-lang/lania-g/protocol/grpc/v3/binding"
	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/di"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	"github.com/sao-lang/lania-g/kernel/v3/registry"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	grpcprotocol "github.com/sao-lang/lania-g/protocol/grpc/v3/protocol"
)

type testService struct{}

type invalidStreamService struct{}

type echoCompositeArgs struct {
	HC     *runtime.HandlerContext
	Ctx    stdctx.Context
	Req    *emptypb.Empty             `req:"true" required:"true"`
	Token  string                     `header:"Authorization" required:"true"`
	Wrap   grpcbinding.Header[string] `header:"Authorization"`
	Full   grpcbinding.FullMethod
	Meta   grpcbinding.Metadata
	Route  grpcbinding.Method
	Svc    grpcbinding.Service
}

type watchCompositeArgs struct {
	Req    *emptypb.Empty                            `req:"true" required:"true"`
	Stream grpcbinding.ServerStream[*emptypb.Empty]
	Raw    grpcbinding.RawServerStream
}

type uploadCompositeArgs struct {
	Stream grpcbinding.ClientStream[*emptypb.Empty]
	Raw    grpcbinding.RawServerStream
}

type chatCompositeArgs struct {
	Stream grpcbinding.BidiStream[*emptypb.Empty, *emptypb.Empty]
	Raw    grpcbinding.RawServerStream
}

type invalidClientCompositeArgs struct {
	Req    *emptypb.Empty                            `req:"true" required:"true"`
	Stream grpcbinding.ClientStream[*emptypb.Empty]
}

func (s *testService) Echo(
	ctx stdctx.Context,
	req *emptypb.Empty,
	md grpcbinding.Metadata,
	token grpcbinding.Header[string],
	full grpcbinding.FullMethod,
) (*emptypb.Empty, error) {
	// Echo 保留“逐参数 + BindParam”这条兼容路径，避免新增 CompositeStruct 后回归老用户。
	if ctx == nil {
		return nil, stdctx.Canceled
	}
	if string(full) != "/TestService/Echo" {
		return nil, stdctx.Canceled
	}
	if len(md["authorization"]) == 0 || md["authorization"][0] != "tkn" {
		return nil, stdctx.Canceled
	}
	if token.Value != "tkn" {
		return nil, stdctx.Canceled
	}
	return req, nil
}

func (s *testService) EchoComposite(args echoCompositeArgs) (*emptypb.Empty, error) {
	// EchoComposite 覆盖新的推荐路径：一个 composite 参数里同时声明 req/header/context/meta。
	if args.HC == nil {
		return nil, stdctx.Canceled
	}
	if args.Ctx == nil {
		return nil, stdctx.Canceled
	}
	if args.Req == nil {
		return nil, stdctx.Canceled
	}
	if args.Token != "tkn" || args.Wrap.Value != "tkn" {
		return nil, stdctx.Canceled
	}
	if string(args.Full) != "/TestService/EchoComposite" {
		return nil, stdctx.Canceled
	}
	if string(args.Route) != "EchoComposite" || string(args.Svc) != "TestService" {
		return nil, stdctx.Canceled
	}
	if len(args.Meta["authorization"]) == 0 || args.Meta["authorization"][0] != "tkn" {
		return nil, stdctx.Canceled
	}
	return args.Req, nil
}

type testRepo struct {
	ID string
}

func (s *testService) EchoDI(repo *testRepo, req *emptypb.Empty) (*emptypb.Empty, error) {
	// 这里应该来自 DI，不应被 `RequestMessage` binding 吞掉。
	if repo == nil {
		return nil, stdctx.Canceled
	}
	return req, nil
}

func (s *testService) EchoHeaderNoKey(h grpcbinding.Header[string]) (*emptypb.Empty, error) {
	// 这个 case 继续验证老的 Header[T] 直注入路径在缺少 binding name 时仍会给出清晰错误。
	_ = h
	return &emptypb.Empty{}, nil
}

func (s *testService) Watch(req *emptypb.Empty, stream grpcbinding.ServerStream[*emptypb.Empty]) error {
	if req == nil {
		return stdctx.Canceled
	}
	if err := stream.Send(&emptypb.Empty{}); err != nil {
		return err
	}
	return stream.Send(&emptypb.Empty{})
}

func (s *testService) WatchComposite(args watchCompositeArgs) error {
	if args.Req == nil || args.Raw.ServerStream == nil {
		return stdctx.Canceled
	}
	if err := args.Stream.Send(&emptypb.Empty{}); err != nil {
		return err
	}
	return args.Stream.Send(&emptypb.Empty{})
}

func (s *testService) Upload(stream grpcbinding.ClientStream[*emptypb.Empty]) (*emptypb.Empty, error) {
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return &emptypb.Empty{}, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func (s *testService) UploadComposite(args uploadCompositeArgs) (*emptypb.Empty, error) {
	if args.Raw.ServerStream == nil {
		return nil, stdctx.Canceled
	}
	for {
		_, err := args.Stream.Recv()
		if err == io.EOF {
			return &emptypb.Empty{}, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func (s *testService) Chat(stream grpcbinding.BidiStream[*emptypb.Empty, *emptypb.Empty]) error {
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := stream.Send(&emptypb.Empty{}); err != nil {
			return err
		}
	}
}

func (s *testService) ChatComposite(args chatCompositeArgs) error {
	if args.Raw.ServerStream == nil {
		return stdctx.Canceled
	}
	for {
		_, err := args.Stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := args.Stream.Send(&emptypb.Empty{}); err != nil {
			return err
		}
	}
}

func (s *invalidStreamService) Upload(req *emptypb.Empty, stream grpcbinding.ClientStream[*emptypb.Empty]) (*emptypb.Empty, error) {
	return req, nil
}

func (s *invalidStreamService) UploadComposite(args invalidClientCompositeArgs) (*emptypb.Empty, error) {
	_ = args
	return &emptypb.Empty{}, nil
}

type testHost struct {
	rt  *runtime.Runtime
	reg *registry.Registry
}

func (h *testHost) Runtime() *runtime.Runtime          { return h.rt }
func (h *testHost) Registry() *registry.Registry       { return h.reg }
func (h *testHost) ModuleRef() *module.ModuleRef       { return nil }
var _ adapter.Host = (*testHost)(nil)

type fakeServerStream struct {
	ctx      stdctx.Context
	recv     []any
	recvIdx  int
	sent     []any
	trailers grpcmetadata.MD
}

func (s *fakeServerStream) SetHeader(md grpcmetadata.MD) error { return nil }
func (s *fakeServerStream) SendHeader(md grpcmetadata.MD) error { return nil }
func (s *fakeServerStream) SetTrailer(md grpcmetadata.MD)       { s.trailers = md }
func (s *fakeServerStream) Context() stdctx.Context {
	if s.ctx == nil {
		return stdctx.Background()
	}
	return s.ctx
}
func (s *fakeServerStream) SendMsg(m any) error {
	s.sent = append(s.sent, m)
	return nil
}
func (s *fakeServerStream) RecvMsg(m any) error {
	if s.recvIdx >= len(s.recv) {
		return io.EOF
	}
	// 这个 fake stream 只实现当前测试需要的最小行为：
	// 按顺序把预置消息拷贝进 grpc 解码目标，用来模拟真实 streaming transport 的收包过程。
	src := s.recv[s.recvIdx]
	s.recvIdx++
	dst := reflect.ValueOf(m)
	srcVal := reflect.ValueOf(src)
	if dst.Kind() != reflect.Ptr || !dst.Elem().CanSet() {
		return io.ErrUnexpectedEOF
	}
	if srcVal.Type().AssignableTo(dst.Type()) {
		dst.Elem().Set(srcVal.Elem())
		return nil
	}
	if srcVal.Type().AssignableTo(dst.Elem().Type()) {
		dst.Elem().Set(srcVal)
		return nil
	}
	if srcVal.Kind() == reflect.Ptr && srcVal.Elem().Type().AssignableTo(dst.Elem().Type()) {
		dst.Elem().Set(srcVal.Elem())
		return nil
	}
	return io.ErrUnexpectedEOF
}

func TestPluginScan_RequiresExplicitRegistry(t *testing.T) {
	svc := &testService{}
	pSvc, _ := di.ProviderFromInstance(svc, di.Singleton)
	root := module.CreateModule(nil, []*di.Provider{pSvc}, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)

	reg := registry.New()
	NewAPI(reg).Service("TestService", svc).Method("Echo", svc.Echo).BindParam(3, "Authorization").Build()

	_, err := (&Plugin{}).Scan(moduleRef, nil)
	if err == nil {
		t.Fatalf("expected missing registry error")
	}
	if !strings.Contains(err.Error(), "requires an explicit registry") {
		t.Fatalf("unexpected error: %v", err)
	}

	compiled, err := compiler.Compile(moduleRef, reg, NewPlugin())
	if err != nil {
		t.Fatalf("compile with explicit registry: %v", err)
	}
	if compiled == nil {
		t.Fatalf("expected compiled app")
	}
}

func TestPlugin_CompileAndExecute(t *testing.T) {
	svc := &testService{}
	pSvc, _ := di.ProviderFromInstance(svc, di.Singleton)
	pRepo, _ := di.ProviderFromInstance(&testRepo{ID: "ok"}, di.Singleton)
	root := module.CreateModule(nil, []*di.Provider{pSvc, pRepo}, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)

	reg := registry.New()
	b := NewAPI(reg).Service("TestService", svc)
	b.Method("Echo", svc.Echo).BindParam(3, "Authorization") // should be normalized to lower-case
	b.Method("EchoComposite", svc.EchoComposite)
	b.Method("EchoDI", svc.EchoDI)
	b.Method("EchoHeaderNoKey", svc.EchoHeaderNoKey)
	b.Build()

	compiled, err := compiler.Compile(moduleRef, reg, NewPlugin())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	rt := runtime.NewRuntime()
	if err := compiled.Install(rt); err != nil {
		t.Fatalf("install: %v", err)
	}

	ctx := runtime.NewHandlerContext(grpcprotocol.Protocol)
	ctx.WithContext(stdctx.Background())
	ctx.Request.Method = "Echo"
	ctx.Request.Path = "TestService"
	ctx.Request.Body = &emptypb.Empty{}
	ctx.Set(grpcbinding.MetadataKeyIncomingMetadata, grpcmetadata.MD{"authorization": []string{"tkn"}})
	ctx.Set(grpcbinding.MetadataKeyFullMethod, "/TestService/Echo")

	out, err := rt.Execute(ctx)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, ok := out.(*emptypb.Empty); !ok {
		t.Fatalf("out type=%T want *emptypb.Empty", out)
	}

	{
		ctx := runtime.NewHandlerContext(grpcprotocol.Protocol)
		ctx.WithContext(stdctx.Background())
		ctx.Request.Method = "EchoComposite"
		ctx.Request.Path = "TestService"
		ctx.Request.Body = &emptypb.Empty{}
		ctx.Set(grpcbinding.MetadataKeyIncomingMetadata, grpcmetadata.MD{"authorization": []string{"tkn"}})
		ctx.Set(grpcbinding.MetadataKeyFullMethod, "/TestService/EchoComposite")
		ctx.Set(grpcbinding.MetadataKeyService, "TestService")
		ctx.Set(grpcbinding.MetadataKeyMethod, "EchoComposite")
		out, err := rt.Execute(ctx)
		if err != nil {
			t.Fatalf("execute EchoComposite: %v", err)
		}
		if _, ok := out.(*emptypb.Empty); !ok {
			t.Fatalf("out type=%T want *emptypb.Empty", out)
		}
	}

	{
		ctx := runtime.NewHandlerContext(grpcprotocol.Protocol)
		ctx.WithContext(stdctx.Background())
		ctx.Request.Method = "EchoComposite"
		ctx.Request.Path = "TestService"
		ctx.Request.Body = &emptypb.Empty{}
		ctx.Set(grpcbinding.MetadataKeyFullMethod, "/TestService/EchoComposite")
		ctx.Set(grpcbinding.MetadataKeyService, "TestService")
		ctx.Set(grpcbinding.MetadataKeyMethod, "EchoComposite")
		_, err := rt.Execute(ctx)
		if err == nil {
			t.Fatalf("execute EchoComposite without header: want error")
		}
	}

	{
		ctx := runtime.NewHandlerContext(grpcprotocol.Protocol)
		ctx.WithContext(stdctx.Background())
		ctx.Request.Method = "EchoDI"
		ctx.Request.Path = "TestService"
		ctx.Request.Body = &emptypb.Empty{}
		out, err := rt.Execute(ctx)
		if err != nil {
			t.Fatalf("execute EchoDI: %v", err)
		}
		if _, ok := out.(*emptypb.Empty); !ok {
			t.Fatalf("out type=%T want *emptypb.Empty", out)
		}
	}

	{
		ctx := runtime.NewHandlerContext(grpcprotocol.Protocol)
		ctx.WithContext(stdctx.Background())
		ctx.Request.Method = "EchoHeaderNoKey"
		ctx.Request.Path = "TestService"
		ctx.Request.Body = &emptypb.Empty{}
		_, err := rt.Execute(ctx)
		if err == nil {
			t.Fatalf("execute EchoHeaderNoKey: want error")
		}
	}
}

func TestPlugin_CompileWithControllerOwnedReceiver(t *testing.T) {
	svc := &testService{}
	root := module.CreateModule(nil, nil, []any{svc}, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)

	reg := registry.New()
	NewAPI(reg).Service("TestService", svc).Method("Echo", svc.Echo).BindParam(3, "Authorization").Build()

	compiled, err := compiler.Compile(moduleRef, reg, NewPlugin())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	rt := runtime.NewRuntime()
	if err := compiled.Install(rt); err != nil {
		t.Fatalf("install: %v", err)
	}

	ctx := runtime.NewHandlerContext(grpcprotocol.Protocol)
	ctx.WithContext(stdctx.Background())
	ctx.Request.Method = "Echo"
	ctx.Request.Path = "TestService"
	ctx.Request.Body = &emptypb.Empty{}
	ctx.Set(grpcbinding.MetadataKeyIncomingMetadata, grpcmetadata.MD{"authorization": []string{"tkn"}})
	ctx.Set(grpcbinding.MetadataKeyFullMethod, "/TestService/Echo")

	out, err := rt.Execute(ctx)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, ok := out.(*emptypb.Empty); !ok {
		t.Fatalf("out type=%T want *emptypb.Empty", out)
	}
}

func TestAdapter_RegisterDeclaredServicesIncludesStreams(t *testing.T) {
	svc := &testService{}
	reg := registry.New()
	NewAPI(reg).Service("TestService", svc).
		Method("Echo", svc.Echo).
		ServerStreamMethod("Watch", svc.Watch).
		ClientStreamMethod("Upload", svc.Upload).
		BidiStreamMethod("Chat", svc.Chat).
		Build()

	rt := runtime.NewRuntime()
	a := &Adapter{host: &testHost{rt: rt, reg: reg}}
	server := grpc.NewServer()
	if err := a.registerDeclaredServices(server); err != nil {
		t.Fatalf("register declared services: %v", err)
	}

	// grpc.Server 内部最终会把 unary 和三种 streaming 都投影进同一个 ServiceInfo 视图；
	// 这里用 IsClientStream / IsServerStream 组合来断言 mode -> StreamDesc 标记的映射是否正确。
	info := server.GetServiceInfo()["TestService"]
	if len(info.Methods) != 4 {
		t.Fatalf("registered method count = %d, want 4", len(info.Methods))
	}
	modes := map[string][2]bool{}
	for _, method := range info.Methods {
		modes[method.Name] = [2]bool{method.IsClientStream, method.IsServerStream}
	}
	if got := modes["Echo"]; got != [2]bool{false, false} {
		t.Fatalf("Echo stream flags = %v", got)
	}
	if got := modes["Watch"]; got != [2]bool{false, true} {
		t.Fatalf("Watch stream flags = %v", got)
	}
	if got := modes["Upload"]; got != [2]bool{true, false} {
		t.Fatalf("Upload stream flags = %v", got)
	}
	if got := modes["Chat"]; got != [2]bool{true, true} {
		t.Fatalf("Chat stream flags = %v", got)
	}
}

func TestAdapter_DispatchStreamModes(t *testing.T) {
	svc := &testService{}
	pSvc, _ := di.ProviderFromInstance(svc, di.Singleton)
	root := module.CreateModule(nil, []*di.Provider{pSvc}, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)

	reg := registry.New()
	NewAPI(reg).Service("TestService", svc).
		ServerStreamMethod("Watch", svc.Watch).
		ServerStreamMethod("WatchComposite", svc.WatchComposite).
		ClientStreamMethod("Upload", svc.Upload).
		ClientStreamMethod("UploadComposite", svc.UploadComposite).
		BidiStreamMethod("Chat", svc.Chat).
		BidiStreamMethod("ChatComposite", svc.ChatComposite).
		Build()

	compiled, err := compiler.Compile(moduleRef, reg, NewPlugin())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	rt := runtime.NewRuntime()
	if err := compiled.Install(rt); err != nil {
		t.Fatalf("install: %v", err)
	}

	a := &Adapter{host: &testHost{rt: rt, reg: reg}}

	serverReqType, err := inferMethodRequestType(&MethodDefinition{
		Service:     "TestService",
		Method:      "Watch",
		Mode:        RPCModeServerStream,
		Receiver:    svc,
		HandlerName: "Watch",
	})
	if err != nil {
		t.Fatalf("infer server stream request type: %v", err)
	}
	// server-streaming 要先消费一次首请求消息，再让业务持续 Send 多条结果。
	serverHandler := a.makeStreamHandler(
		runtime.BuildRouteKey(grpcprotocol.Protocol, "Watch", "TestService"),
		"/TestService/Watch",
		"TestService",
		"Watch",
		RPCModeServerStream,
		serverReqType,
	)
	serverStream := &fakeServerStream{
		ctx:  grpcmetadata.NewIncomingContext(stdctx.Background(), grpcmetadata.Pairs("authorization", "tkn")),
		recv: []any{&emptypb.Empty{}},
	}
	if err := serverHandler(a, serverStream); err != nil {
		t.Fatalf("server stream handler: %v", err)
	}
	if len(serverStream.sent) != 2 {
		t.Fatalf("server stream sends = %d, want 2", len(serverStream.sent))
	}

	serverCompositeReqType, err := inferMethodRequestType(&MethodDefinition{
		Service:     "TestService",
		Method:      "WatchComposite",
		Mode:        RPCModeServerStream,
		Receiver:    svc,
		HandlerName: "WatchComposite",
	})
	if err != nil {
		t.Fatalf("infer composite server stream request type: %v", err)
	}
	serverCompositeHandler := a.makeStreamHandler(
		runtime.BuildRouteKey(grpcprotocol.Protocol, "WatchComposite", "TestService"),
		"/TestService/WatchComposite",
		"TestService",
		"WatchComposite",
		RPCModeServerStream,
		serverCompositeReqType,
	)
	serverCompositeStream := &fakeServerStream{
		ctx:  grpcmetadata.NewIncomingContext(stdctx.Background(), grpcmetadata.Pairs("authorization", "tkn")),
		recv: []any{&emptypb.Empty{}},
	}
	if err := serverCompositeHandler(a, serverCompositeStream); err != nil {
		t.Fatalf("server composite stream handler: %v", err)
	}
	if len(serverCompositeStream.sent) != 2 {
		t.Fatalf("server composite stream sends = %d, want 2", len(serverCompositeStream.sent))
	}

	// client-streaming 不依赖首请求体；业务通过 ClientStream.Recv() 自己循环拉取所有输入，
	// transport 只在 handler 返回后回写一次聚合结果。
	clientHandler := a.makeStreamHandler(
		runtime.BuildRouteKey(grpcprotocol.Protocol, "Upload", "TestService"),
		"/TestService/Upload",
		"TestService",
		"Upload",
		RPCModeClientStream,
		nil,
	)
	clientStream := &fakeServerStream{
		ctx:  stdctx.Background(),
		recv: []any{&emptypb.Empty{}, &emptypb.Empty{}},
	}
	if err := clientHandler(a, clientStream); err != nil {
		t.Fatalf("client stream handler: %v", err)
	}
	if len(clientStream.sent) != 1 {
		t.Fatalf("client stream responses = %d, want 1", len(clientStream.sent))
	}

	clientCompositeHandler := a.makeStreamHandler(
		runtime.BuildRouteKey(grpcprotocol.Protocol, "UploadComposite", "TestService"),
		"/TestService/UploadComposite",
		"TestService",
		"UploadComposite",
		RPCModeClientStream,
		nil,
	)
	clientCompositeStream := &fakeServerStream{
		ctx:  stdctx.Background(),
		recv: []any{&emptypb.Empty{}, &emptypb.Empty{}},
	}
	if err := clientCompositeHandler(a, clientCompositeStream); err != nil {
		t.Fatalf("client composite stream handler: %v", err)
	}
	if len(clientCompositeStream.sent) != 1 {
		t.Fatalf("client composite stream responses = %d, want 1", len(clientCompositeStream.sent))
	}

	// bidi-streaming 的关键在于 transport 不再干预收发节奏，
	// handler 内部可以按任意顺序交替 Recv / Send。
	bidiHandler := a.makeStreamHandler(
		runtime.BuildRouteKey(grpcprotocol.Protocol, "Chat", "TestService"),
		"/TestService/Chat",
		"TestService",
		"Chat",
		RPCModeBidiStream,
		nil,
	)
	bidiStream := &fakeServerStream{
		ctx:  stdctx.Background(),
		recv: []any{&emptypb.Empty{}, &emptypb.Empty{}},
	}
	if err := bidiHandler(a, bidiStream); err != nil {
		t.Fatalf("bidi stream handler: %v", err)
	}
	if len(bidiStream.sent) != 2 {
		t.Fatalf("bidi stream sends = %d, want 2", len(bidiStream.sent))
	}

	bidiCompositeHandler := a.makeStreamHandler(
		runtime.BuildRouteKey(grpcprotocol.Protocol, "ChatComposite", "TestService"),
		"/TestService/ChatComposite",
		"TestService",
		"ChatComposite",
		RPCModeBidiStream,
		nil,
	)
	bidiCompositeStream := &fakeServerStream{
		ctx:  stdctx.Background(),
		recv: []any{&emptypb.Empty{}, &emptypb.Empty{}},
	}
	if err := bidiCompositeHandler(a, bidiCompositeStream); err != nil {
		t.Fatalf("bidi composite stream handler: %v", err)
	}
	if len(bidiCompositeStream.sent) != 2 {
		t.Fatalf("bidi composite stream sends = %d, want 2", len(bidiCompositeStream.sent))
	}
}

func TestPlugin_CompileRejectsInvalidClientStreamSignature(t *testing.T) {
	svc := &invalidStreamService{}
	pSvc, _ := di.ProviderFromInstance(svc, di.Singleton)
	root := module.CreateModule(nil, []*di.Provider{pSvc}, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)

	reg := registry.New()
	NewAPI(reg).Service("InvalidService", svc).
		ClientStreamMethod("Upload", svc.Upload).
		Build()

	_, err := compiler.Compile(moduleRef, reg, NewPlugin())
	if err == nil {
		t.Fatalf("expected compile error")
	}
	// 这里验证的是“把 request 参数写进 client-stream handler”会在编译期被拦截，
	// 而不是等到运行期 binding 才给出一个更难定位的错误。
	if !strings.Contains(err.Error(), "cannot declare Req[T] or request message parameters") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPlugin_CompileRejectsInvalidClientStreamCompositeSignature(t *testing.T) {
	svc := &invalidStreamService{}
	pSvc, _ := di.ProviderFromInstance(svc, di.Singleton)
	root := module.CreateModule(nil, []*di.Provider{pSvc}, nil, nil, nil)
	if err := root.Init(); err != nil {
		t.Fatalf("init module: %v", err)
	}
	moduleRef := module.NewModuleRef(root)

	reg := registry.New()
	NewAPI(reg).Service("InvalidService", svc).
		ClientStreamMethod("UploadComposite", svc.UploadComposite).
		Build()

	_, err := compiler.Compile(moduleRef, reg, NewPlugin())
	if err == nil {
		t.Fatalf("expected compile error")
	}
	if !strings.Contains(err.Error(), "cannot declare Req[T] or request message parameters") {
		t.Fatalf("unexpected error: %v", err)
	}
}
