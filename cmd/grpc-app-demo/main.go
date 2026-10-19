package main

import (
	stdctx "context"
	"io"
	"log"
	"os"

	grpcadapter "github.com/sao-lang/lania-g/protocol/grpc/v3"
	"github.com/sao-lang/lania-g/application/v3"
	"github.com/sao-lang/lania-g/cmd/internal/addr"
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

// Declare 声明 gRPC service。
func (s *UserService) Declare(r *grpcadapter.Routes) {
	r.Service("UserService").
		Method("Ping", s.Ping).
		ServerStreamMethod("Watch", s.Watch).
		ClientStreamMethod("Upload", s.Upload).
		BidiStreamMethod("Chat", s.Chat)
}

func main() {
	userSvc := &UserService{}

	grpcModule := grpcadapter.Module(grpcadapter.Config{Addr: addr.GRPC(":50051")})
	root := module.New(module.Metadata{
		Imports:     []module.Module{grpcModule},
		Controllers: []any{userSvc},
	})

	app, err := application.New(root, application.Options{Logger: os.Stdout})
	if err != nil {
		log.Fatal(err)
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
