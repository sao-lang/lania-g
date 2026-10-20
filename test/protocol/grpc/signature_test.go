package grpc_test

// 测试位于仓库根目录 test/，通过 dot-import 访问被测包。
import . "github.com/sao-lang/lania-g/protocol/grpc/v3"
import (
	"reflect"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
	"google.golang.org/protobuf/types/known/emptypb"
)

type emptyRequestService struct{}

// Me 只依赖上下文（没有请求消息字段），用于验证无请求消息时的默认推断。
func (s *emptyRequestService) Me(_ *runtime.HandlerContext) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func TestInferMethodRequestType_DefaultsToEmptyWhenNoRequestField(t *testing.T) {
	def := &MethodDefinition{
		Service:     "UserService",
		Method:      "Me",
		Mode:        RPCModeUnary,
		Receiver:    &emptyRequestService{},
		HandlerName: "Me",
	}

	got, err := InferMethodRequestType(def)
	if err != nil {
		t.Fatalf("infer request type: %v", err)
	}
	if got != reflect.TypeFor[*emptypb.Empty]() {
		t.Fatalf("request type = %v, want *emptypb.Empty", got)
	}
}

func TestInferMethodRequestType_ExplicitRequestTypeWins(t *testing.T) {
	def := &MethodDefinition{
		Service:     "UserService",
		Method:      "Me",
		Mode:        RPCModeUnary,
		Receiver:    &emptyRequestService{},
		HandlerName: "Me",
		RequestType: reflect.TypeFor[*emptypb.Empty](),
	}

	got, err := InferMethodRequestType(def)
	if err != nil {
		t.Fatalf("infer request type: %v", err)
	}
	if got != reflect.TypeFor[*emptypb.Empty]() {
		t.Fatalf("request type = %v, want *emptypb.Empty", got)
	}
}
