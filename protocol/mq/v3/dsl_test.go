package mq

import (
	"strings"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

type invalidConsumer struct{}

func (c *invalidConsumer) Handle() {}

func TestConsumerBuilder_BuildE_ValidatesSubscriptionHandler(t *testing.T) {
	api := NewAPI(registry.New())
	consumer := &invalidConsumer{}

	_, err := api.Consumer("default", consumer).On("user.created", func() {}).BuildE()
	if err == nil {
		t.Fatalf("BuildE: want error")
	}
	if !strings.Contains(err.Error(), "invalid mq subscription declaration") {
		t.Fatalf("unexpected error: %v", err)
	}
}
