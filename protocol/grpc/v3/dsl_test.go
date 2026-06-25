package grpc

import (
	"context"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

type testGRPCService struct{}

func (s *testGRPCService) Get(ctx context.Context) error {
	_ = ctx
	return nil
}

func TestServiceBuilder_BuildEOnScopeMisuse(t *testing.T) {
	reg := registry.New()
	svc := &testGRPCService{}
	builder := NewAPI(reg).Service("UserService", svc)
	builder.Method("Get", svc.Get)
	builder.UseGuards("auth")

	if builder.Err() == nil {
		t.Fatalf("expected builder error")
	}
	defs, err := builder.BuildE()
	if err == nil {
		t.Fatalf("expected build error")
	}
	if defs != nil {
		t.Fatalf("defs should be nil")
	}
}
