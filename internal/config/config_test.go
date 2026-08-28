package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadHexInputs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("monitor:\n  bus: 15\ninputs:\n  mac: 0x11\n  linux: 0x0f\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Monitor.Bus != 15 || cfg.Inputs["mac"] != 0x11 || cfg.Inputs["linux"] != 0x0f {
		t.Fatalf("Load() = %#v", cfg)
	}
}
