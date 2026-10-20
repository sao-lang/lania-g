package grpc_test

import . "github.com/sao-lang/lania-g/integrations/grpc/v3"
import (
	stdctx "context"
	"io"
	"net"
	"testing"

	gogrpc "google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

func newTestConfig(t *testing.T) (Config, func()) {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := gogrpc.NewServer()
	go func() {
		_ = server.Serve(listener)
	}()

	cfg := Config{
		Target:   "bufnet",
		Insecure: true,
		DialOptions: []gogrpc.DialOption{
			gogrpc.WithContextDialer(func(ctx stdctx.Context, target string) (net.Conn, error) {
				return listener.Dial()
			}),
		},
	}
	cleanup := func() {
		server.Stop()
		_ = listener.Close()
	}
	return cfg, cleanup
}

func newStreamTestConfig(t *testing.T) (Config, func()) {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := gogrpc.NewServer()
	server.RegisterService(&gogrpc.ServiceDesc{
		ServiceName: "test.StreamService",
		HandlerType: (*any)(nil),
		Streams: []gogrpc.StreamDesc{
			{
				StreamName:    "Echo",
				ClientStreams: true,
				ServerStreams: true,
				Handler: func(srv any, stream gogrpc.ServerStream) error {
					for {
						var msg emptypb.Empty
						err := stream.RecvMsg(&msg)
						if err == io.EOF {
							return nil
						}
						if err != nil {
							return err
						}
						if err := stream.SendMsg(&emptypb.Empty{}); err != nil {
							return err
						}
					}
				},
			},
		},
	}, nil)
	go func() {
		_ = server.Serve(listener)
	}()

	cfg := Config{
		Target:   "bufnet-stream",
		Insecure: true,
		DialOptions: []gogrpc.DialOption{
			gogrpc.WithContextDialer(func(ctx stdctx.Context, target string) (net.Conn, error) {
				return listener.Dial()
			}),
		},
	}
	cleanup := func() {
		server.Stop()
		_ = listener.Close()
	}
	return cfg, cleanup
}

func TestClient_NewStream(t *testing.T) {
	cfg, cleanup := newStreamTestConfig(t)
	defer cleanup()

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer client.Close()

	stream, err := client.NewStream(stdctx.Background(), &gogrpc.StreamDesc{
		StreamName:    "Echo",
		ClientStreams: true,
		ServerStreams: true,
	}, "/test.StreamService/Echo")
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	if err := stream.SendMsg(&emptypb.Empty{}); err != nil {
		t.Fatalf("send msg: %v", err)
	}
	var out emptypb.Empty
	if err := stream.RecvMsg(&out); err != nil {
		t.Fatalf("recv msg: %v", err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("close send: %v", err)
	}
}
