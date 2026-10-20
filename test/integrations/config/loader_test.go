package config_test

import . "github.com/sao-lang/lania-g/integrations/config/v3"

import (
	"os"
	"path/filepath"
	"testing"
)

type fileLoadConfig struct {
	App struct {
		Name string `json:"name"`
	} `json:"app"`
}

func TestLoad_ReadsConfigIntoStruct(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.json")
	if err := os.WriteFile(path, []byte(`{"app":{"name":"demo"}}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var cfg fileLoadConfig
	if err := Load(path, &cfg); err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.App.Name != "demo" {
		t.Fatalf("app.name = %q, want demo", cfg.App.Name)
	}
}
