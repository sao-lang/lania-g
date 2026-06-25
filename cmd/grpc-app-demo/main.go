package main

import (
	stdctx "context"
	"io"
	"os"

	grpcadapter "github.com/sao-lang/lania-g/protocol/grpc/v3"
	"github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/kernel/v3/module"
	grpcbinding "github.com/sao-lang/lania-g/protocol/grpc/v3/binding"

	"google.golang.org/protobuf/types/known/emptypb"
)

type UserService struct{}

type pingArgs struct {
	Ctx stdctx.Context
	Req *emptypb.Empty `req:"true" required:"true"`
}

type watchArgs struct {
	Req    *emptypb.Empty                         `req:"true" required:"true"`
	Stream grpcbinding.ServerStream[*emptypb.Empty]
}

type uploadArgs struct {
	Stream grpcbinding.ClientStream[*emptypb.Empty]
	Raw    grpcbinding.RawServerStream
}

type chatArgs struct {
	Stream grpcbinding.BidiStream[*emptypb.Empty, *emptypb.Empty]
}

func (s *UserService) Ping(args pingArgs) (*emptypb.Empty, error) {
	_ = args.Ctx
	return args.Req, nil
}

func (s *UserService) Watch(args watchArgs) error {
	_ = args.Req
	return args.Stream.Send(&emptypb.Empty{})
}

func (s *UserService) Upload(args uploadArgs) (*emptypb.Empty, error) {
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

func (s *UserService) Chat(args chatArgs) error {
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

func main() {
	userSvc := &UserService{}

	// gRPC receiver 只需要明确归属到某个模块 owner；业务样板中直接放入模块 owner 槽位即可。
	root := module.CreateModule(nil, nil, []any{userSvc}, nil, nil)

	grpcAdapter := grpcadapter.New(":50051")
	app, err := application.NewWithOptions(root, application.Options{
		Registry:        application.NewRegistry(),
		StartupReporter: os.Stdout,
	}, grpcAdapter)
	if err != nil {
		panic(err)
	}

	grpcAPI := grpcAdapter.API().(*grpcadapter.API)
	builder := grpcAPI.Service("UserService", userSvc)
	builder.Method("Ping", userSvc.Ping)
	builder.ServerStreamMethod("Watch", userSvc.Watch)
	builder.ClientStreamMethod("Upload", userSvc.Upload)
	builder.BidiStreamMethod("Chat", userSvc.Chat)
	builder.Build()

	if _, err := app.CompileDiagnostics(); err != nil {
		panic(err)
	}
	if _, err := app.StartupReport(); err != nil {
		panic(err)
	}
	if err := app.Run(); err != nil {
		panic(err)
	}
}
