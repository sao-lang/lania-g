package ws

import (
	"strings"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

type invalidGateway struct{}

func (g *invalidGateway) Handle() {}

func TestGatewayBuilder_BuildE_ValidatesHandler(t *testing.T) {
	api := NewAPI(registry.New())
	gateway := &invalidGateway{}

	_, err := api.Gateway("/chat", gateway).On("joined", func() {}).BuildE()
	if err == nil {
		t.Fatalf("BuildE: want error")
	}
	if !strings.Contains(err.Error(), "invalid ws handler declaration") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandlerBuilder_On_CanChain(t *testing.T) {
	api := NewAPI(registry.New())
	gateway := &testGateway{}

	_, err := api.Gateway("/ws/users", gateway).
		On("echo", gateway.Echo).
		On("echo_auto", gateway.EchoAutoStruct).
		BuildE()
	if err != nil {
		t.Fatalf("BuildE: %v", err)
	}
}
