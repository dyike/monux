package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dyike/monux/internal/monitor"
)

func TestLoadHexInputs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("monitor:\n  id: 15\ninputs:\n  mac: 0x11\n  linux: 0x0f\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Monitor.ID != "15" || cfg.Inputs["mac"] != 0x11 || cfg.Inputs["linux"] != 0x0f {
		t.Fatalf("Load() = %#v", cfg)
	}
}

func TestLoadNamedConnectorInputs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("monitor:\n  id: 23\ninputs:\n  mac: hdmi-1\n  linux: displayport-1\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Inputs["mac"] != 0x11 || cfg.Inputs["linux"] != 0x0f {
		t.Fatalf("Load() = %#v", cfg)
	}
}

func TestSaveCreatesLoadableHexConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	cfg := Config{
		Monitor: MonitorConfig{ID: "23"},
		Inputs: map[string]monitor.Input{
			"mac":   0x11,
			"linux": 0x0f,
		},
	}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`id: "23"`, "linux: displayport-1 # DDC 0x0f", "mac: hdmi-1 # DDC 0x11"} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated config does not contain %q:\n%s", want, text)
		}
	}
	if strings.Index(text, "linux:") > strings.Index(text, "mac:") {
		t.Fatalf("input names are not sorted:\n%s", text)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() generated config error = %v", err)
	}
	if loaded.Monitor.ID != "23" || loaded.Inputs["linux"] != 0x0f || loaded.Inputs["mac"] != 0x11 {
		t.Fatalf("Load() generated config = %#v", loaded)
	}
}

func TestSavePreservesExistingPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Monitor: MonitorConfig{ID: "1"}, Inputs: map[string]monitor.Input{"mac": 0x11}}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("mode = %o, want 640", got)
	}
}
