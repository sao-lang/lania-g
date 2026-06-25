// grpc.go 实现 gRPC adapter 的主入口与宿主集成逻辑。
package grpc

import (
	stdctx "context"
	"fmt"
	"net"
	"reflect"
	"strings"

	"github.com/sao-lang/lania-g/kernel/v3/adapter"
	"github.com/sao-lang/lania-g/kernel/v3/compiler"
	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	grpcbinding "github.com/sao-lang/lania-g/protocol/grpc/v3/binding"
	grpcprotocol "github.com/sao-lang/lania-g/protocol/grpc/v3/protocol"
	"google.golang.org/grpc"
	grpcmetadata "google.golang.org/grpc/metadata"
)

// Adapter 是 gRPC 协议的传输层适配器。它负责：
// - 通过插件把 gRPC Service/Method 声明编译为 runtime handler
// - 注册 grpc.ServiceDesc，并把 unary/streaming 请求都转发给 runtime.Execute
// - 启动和停止底层 grpc.Server
type Adapter struct {
	host adapter.Host
	api  *API

	addr string
	opts []grpc.ServerOption

	server   *grpc.Server
	listener net.Listener

	serverProvided bool
	started        bool
	registered     bool
}

// New 创建 gRPC adapter。
// - `New()` 以默认地址 `:50051` 启动独立监听模式
// - `New(":50051")` 以指定地址启动独立监听模式
//
// 注意：gRPC 不参与 `app.Listen(addr)` 的 HTTP 共享端口复用。
// 这是该 adapter 唯一的公开端口绑定入口。
func New(addr ...string) *Adapter {
	a := &Adapter{
		addr: ":50051",
		api:  NewCompatAPI(),
	}
	if len(addr) > 0 && strings.TrimSpace(addr[0]) != "" {
		a.addr = addr[0]
	}
	return a
}

// Listen 仅为兼容旧写法保留。
// Deprecated: 请改用 `grpc.New(":addr")`。
func Listen(addr string) *Adapter { return New(addr) }

// WithAddr 仅为兼容旧写法保留。
// Deprecated: 请改用 `grpc.New(":addr")`。
func (a *Adapter) WithAddr(addr string) *Adapter {
	if strings.TrimSpace(addr) != "" {
		a.addr = addr
	}
	return a
}

// ID 返回 adapter 标识。
func (a *Adapter) ID() string { return AdapterID }

// Plugins 返回 gRPC 协议参与编译的插件列表。
func (a *Adapter) Plugins() []compiler.ProtocolPlugin { return []compiler.ProtocolPlugin{NewPlugin()} }

// API 返回 gRPC adapter 暴露给应用侧的 DSL API。
func (a *Adapter) API() any { return a.api }

// WithServerOptions 为内部创建的 `grpc.Server` 追加 server options。
func (a *Adapter) WithServerOptions(opts ...grpc.ServerOption) *Adapter {
	a.opts = append(a.opts, opts...)
	return a
}

// WithServer 注入一个外部构造好的 `grpc.Server`。
// 如果使用这个入口，Stop 时不会销毁用户显式提供的 server 实例。
func (a *Adapter) WithServer(server *grpc.Server) *Adapter {
	if server != nil {
		a.server = server
		a.serverProvided = true
	}
	return a
}

// Server 返回当前 adapter 正在使用的 `grpc.Server`。
func (a *Adapter) Server() *grpc.Server { return a.server }

// Mount 将 gRPC adapter 挂载到应用 host 上，并把 API 绑定到当前 registry。
func (a *Adapter) Mount(host adapter.Host) error {
	if host == nil {
		return fmt.Errorf("grpc adapter host is nil")
	}
	a.host = host
	a.api = NewAPI(host.Registry())
	return nil
}

// Start 启动 gRPC adapter。
// 它会在启动前把 registry 中声明的服务方法注册到 grpc.Server 上。
//
// 整体顺序是：
// - 确保 server/listener 存在
// - 把 DSL 声明回放成 grpc.ServiceDesc + grpc.MethodDesc
// - 启动底层 grpc.Server 开始收包
func (a *Adapter) Start() error {
	if a.host == nil {
		return fmt.Errorf("grpc adapter not mounted")
	}
	if a.started {
		// 保持 Start 的幂等性：已经启动过时直接返回。
		return nil
	}
	if a.server == nil {
		a.server = grpc.NewServer(a.opts...)
	}
	if a.listener == nil {
		ln, err := net.Listen("tcp", a.addr)
		if err != nil {
			return err
		}
		a.listener = ln
	}

	if !a.registered {
		if err := a.registerDeclaredServices(a.server); err != nil {
			return err
		}
		a.registered = true
	}

	go func() {
		_ = a.server.Serve(a.listener)
	}()
	a.started = true
	return nil
}

// Stop 停止 gRPC adapter，并根据 server 来源决定是否重置内部 server。
func (a *Adapter) Stop() error {
	if a.server != nil {
		a.server.GracefulStop()
	}
	if a.listener != nil {
		_ = a.listener.Close()
		a.listener = nil
	}
	a.started = false
	a.registered = false
	// grpc.Server 不能直接重启；如果不是用户显式注入的，就在这里丢弃，
	// 让后续 Start 能重新创建一个新的 server。
	if !a.serverProvided {
		a.server = nil
	}
	return nil
}

// registerDeclaredServices 把 registry 里的 MethodDefinition 按 service 分组，
// 然后动态注册成 gRPC service。
//
// 这里没有依赖生成代码里的 `RegisterFooServer`，
// 而是统一通过 `grpc.ServiceDesc` 动态桥接到 runtime handler。
func (a *Adapter) registerDeclaredServices(server *grpc.Server) error {
	if a.host == nil || server == nil {
		return nil
	}
	reg := a.host.Registry()
	items := reg.ListDecl(AdapterID, "methods")
	if len(items) == 0 {
		return nil
	}

	byService := make(map[string][]*MethodDefinition)
	seen := make(map[string]map[string]bool) // service -> method -> seen
	for _, item := range items {
		def, ok := item.(*MethodDefinition)
		if !ok || def == nil {
			continue
		}
		if def.Service == "" || def.Method == "" || def.HandlerName == "" || def.Receiver == nil {
			continue
		}
		if seen[def.Service] == nil {
			seen[def.Service] = make(map[string]bool)
		}
		if seen[def.Service][def.Method] {
			return fmt.Errorf("duplicate grpc method declaration: service=%s method=%s", def.Service, def.Method)
		}
		seen[def.Service][def.Method] = true
		byService[def.Service] = append(byService[def.Service], def)
	}

	for service, defs := range byService {
		sd := &grpc.ServiceDesc{
			ServiceName: service,
			// 这里不调用 srv 的真实方法实现，真正的执行入口在 runtime.Execute，
			// 因此 HandlerType 只需要满足 grpc.RegisterService 的签名要求。
			HandlerType: (*any)(nil),
			Methods:     make([]grpc.MethodDesc, 0, len(defs)),
			Streams:     make([]grpc.StreamDesc, 0, len(defs)),
			Metadata:    nil,
		}
		for _, def := range defs {
			def.Mode = def.Mode.Normalize()
			methodName := def.Method
			routeKey := runtime.BuildRouteKey(grpcprotocol.Protocol, def.Method, def.Service)
			fullMethod := "/" + service + "/" + methodName
			switch def.Mode {
			case RPCModeUnary:
				reqType, err := inferMethodRequestType(def)
				if err != nil {
					return err
				}
				sd.Methods = append(sd.Methods, grpc.MethodDesc{
					MethodName: methodName,
					Handler:    a.makeMethodHandler(routeKey, fullMethod, service, methodName, reqType),
				})
			case RPCModeServerStream, RPCModeClientStream, RPCModeBidiStream:
				reqType, err := inferMethodRequestType(def)
				if err != nil {
					return err
				}
				sd.Streams = append(sd.Streams, grpc.StreamDesc{
					StreamName:    methodName,
					Handler:       a.makeStreamHandler(routeKey, fullMethod, service, methodName, def.Mode, reqType),
					ClientStreams: def.Mode.ClientStreams(),
					ServerStreams: def.Mode.ServerStreams(),
				})
			default:
				return fmt.Errorf("unsupported grpc method mode %q for %s/%s", def.Mode, def.Service, def.Method)
			}
		}
		// grpc 框架要求注册时传一个 srv，但我们的 MethodHandler 不会调用它的方法，
		// 所以这里传 adapter 自身作为占位值即可。
		server.RegisterService(sd, a)
	}

	return nil
}

// makeMethodHandler 生成标准 grpc.MethodHandler。
// 这个桥接层负责：
// - 先构造请求消息实例并交给 grpc decoder 填充
// - 再把请求转发给 dispatchUnary
// - 若外部装了 unary interceptor，则按 grpc 约定把它包在最外层
func (a *Adapter) makeMethodHandler(routeKey, fullMethod, service, method string, reqType reflect.Type) grpc.MethodHandler {
	return func(srv any, ctx stdctx.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		in, err := newDecodeTarget(reqType)
		if err != nil {
			return nil, err
		}
		if err := dec(in); err != nil {
			return nil, err
		}

		info := &grpc.UnaryServerInfo{Server: srv, FullMethod: fullMethod}
		handler := func(ctx stdctx.Context, req any) (any, error) {
			return a.dispatchUnary(routeKey, fullMethod, service, method, RPCModeUnary, ctx, req)
		}
		if interceptor == nil {
			return handler(ctx, in)
		}
		return interceptor(ctx, in, info, handler)
	}
}

func (a *Adapter) makeStreamHandler(routeKey, fullMethod, service, method string, mode RPCMode, reqType reflect.Type) grpc.StreamHandler {
	return func(srv any, stream grpc.ServerStream) error {
		return a.dispatchStream(routeKey, fullMethod, service, method, mode.Normalize(), reqType, stream)
	}
}

// dispatchUnary 把一次 gRPC unary 调用投影成框架统一的 HandlerContext，
// 然后直接复用 runtime.Execute。
func (a *Adapter) dispatchUnary(routeKey, fullMethod, service, method string, mode RPCMode, ctx stdctx.Context, req any) (any, error) {
	rctx := runtime.AcquireHandlerContext(grpcprotocol.Protocol)
	defer runtime.ReleaseHandlerContext(rctx)

	if ctx == nil {
		ctx = stdctx.Background()
	}
	rctx.Request.Body = req
	rctx.Request.Raw = req
	a.prepareHandlerContext(rctx, routeKey, fullMethod, service, method, string(mode.Normalize()), ctx)
	return a.host.Runtime().Execute(rctx)
}

func (a *Adapter) dispatchStream(routeKey, fullMethod, service, method string, mode RPCMode, reqType reflect.Type, stream grpc.ServerStream) error {
	rctx := runtime.AcquireHandlerContext(grpcprotocol.Protocol)
	defer runtime.ReleaseHandlerContext(rctx)

	ctx := stdctx.Background()
	if stream != nil && stream.Context() != nil {
		ctx = stream.Context()
	}
	a.prepareHandlerContext(rctx, routeKey, fullMethod, service, method, string(mode), ctx)
	// 对 streaming 而言，真正的收发能力不再来自 Request.Body，
	// 而是通过 Request.Raw 把底层 grpc.ServerStream 暴露给 binding wrapper。
	rctx.Request.Raw = stream

	if mode == RPCModeServerStream {
		// server-streaming 仍然保留“首个请求消息”的语义，便于继续复用现有 request binding。
		in, err := newDecodeTarget(reqType)
		if err != nil {
			return err
		}
		if err := stream.RecvMsg(in); err != nil {
			return err
		}
		rctx.Request.Body = in
	}

	result, err := a.host.Runtime().Execute(rctx)
	if err != nil {
		return err
	}
	if mode == RPCModeClientStream && result != nil {
		// client-streaming 在 handler 完成后只回写一次聚合结果。
		return stream.SendMsg(result)
	}
	return nil
}

// prepareHandlerContext 把 transport 侧的 gRPC 元信息预写入统一 HandlerContext，
// 让后续 binding、诊断和 runtime 执行都继续走现有主路径。
func (a *Adapter) prepareHandlerContext(rctx *runtime.HandlerContext, routeKey, fullMethod, service, method, mode string, ctx stdctx.Context) {
	if rctx == nil {
		return
	}
	if ctx == nil {
		ctx = stdctx.Background()
	}
	rctx.WithContext(ctx)
	rctx.Request.Method = method
	rctx.Request.Path = service
	rctx.RouteKey = routeKey

	if md, ok := grpcmetadata.FromIncomingContext(ctx); ok {
		rctx.Set(grpcbinding.MetadataKeyIncomingMetadata, md)
		for k, values := range md {
			if len(values) > 0 {
				rctx.Request.Headers[k] = values[0]
			}
			rctx.Request.HeadersMulti[k] = append([]string{}, values...)
		}
	}

	rctx.Set(grpcbinding.MetadataKeyFullMethod, fullMethod)
	rctx.Set(grpcbinding.MetadataKeyService, service)
	rctx.Set(grpcbinding.MetadataKeyMethod, method)
	rctx.Set(grpcbinding.MetadataKeyMode, mode)
}

// newDecodeTarget 创建一个能交给 grpc decoder 直接填充的目标对象。
// gRPC unary decoder 需要拿到“请求消息的指针”。
func newDecodeTarget(reqType reflect.Type) (any, error) {
	if reqType == nil {
		return nil, fmt.Errorf("nil grpc request type")
	}
	// grpc dec expects a pointer to the request message.
	if reqType.Kind() == reflect.Ptr {
		return reflect.New(reqType.Elem()).Interface(), nil
	}
	if reqType.Kind() == reflect.Struct {
		return reflect.New(reqType).Interface(), nil
	}
	return nil, fmt.Errorf("unsupported grpc request type: %s", reqType.String())
}

// unwrapReqWrapper 从 `Req[T]` 这种 wrapper 中回推出真实请求消息类型 T。
func unwrapReqWrapper(t reflect.Type) (reflect.Type, bool) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || t.PkgPath() != grpcbinding.PackagePath() {
		return nil, false
	}
	if !strings.HasPrefix(t.Name(), "Req") {
		return nil, false
	}
	field, ok := t.FieldByName("Value")
	if !ok {
		return nil, false
	}
	// Req[T] wrapper itself is passed as value; the actual message type is field.Type.
	return field.Type, true
}

var _ adapter.Adapter = (*Adapter)(nil)
