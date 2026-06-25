package scheduler

import (
	"strings"
	"testing"

	"github.com/sao-lang/lania-g/kernel/v3/registry"
)

type invalidJobService struct{}

func (s *invalidJobService) Run() error { return nil }

func TestJobBuilder_BuildE_ValidatesHandler(t *testing.T) {
	api := NewAPI(registry.New())
	svc := &invalidJobService{}

	_, err := api.Job("cleanup", svc).Every(1, func() {}).BuildE()
	if err == nil {
		t.Fatalf("BuildE: want error")
	}
	if !strings.Contains(err.Error(), "invalid scheduler job declaration") {
		t.Fatalf("unexpected error: %v", err)
	}
}
