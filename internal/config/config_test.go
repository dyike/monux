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
		Node:    NodeConfig{Name: "linux"},
		Peers:   []PeerConfig{{Name: "mac", URL: "http://192.168.5.82:8765", Token: "secret"}},
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
	for _, want := range []string{
		"node:\n  name: linux",
		"peers:\n  - name: mac\n    url: http://192.168.5.82:8765\n    token: secret",
		`id: "23"`, "linux: displayport-1 # DDC 0x0f", "mac: hdmi-1 # DDC 0x11",
	} {
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
	if loaded.Node.Name != "linux" || len(loaded.Peers) != 1 || loaded.Peers[0].URL != "http://192.168.5.82:8765" || loaded.Monitor.ID != "23" || loaded.Inputs["linux"] != 0x0f || loaded.Inputs["mac"] != 0x11 {
		t.Fatalf("Load() generated config = %#v", loaded)
	}
}

func TestValidateRejectsInvalidPeers(t *testing.T) {
	base := Config{Inputs: map[string]monitor.Input{"linux": 0x0f}}
	for _, peer := range []PeerConfig{
		{Name: "", URL: "http://192.168.5.82:8765"},
		{Name: "mac", URL: "192.168.5.82:8765"},
		{Name: "mac", URL: "ftp://192.168.5.82"},
		{Name: "mac", URL: "http://user@192.168.5.82"},
	} {
		cfg := base
		cfg.Peers = []PeerConfig{peer}
		if err := cfg.Validate(); err == nil {
			t.Fatalf("Validate() peer = %#v, error = nil", peer)
		}
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

func TestDefaultPathForPlatform(t *testing.T) {
	tests := []struct {
		platform  string
		home      string
		configDir string
		want      string
	}{
		{"darwin", "/Users/monux", "/Users/monux/Library/Application Support", "/Users/monux/.config/monux/config.yaml"},
		{"linux", "/home/monux", "/home/monux/.config", "/home/monux/.config/monux/config.yaml"},
		{"windows", `C:\\Users\\monux`, `C:\\Users\\monux\\AppData\\Roaming`, `C:\\Users\\monux\\AppData\\Roaming/monux/config.yaml`},
	}
	for _, test := range tests {
		if got := defaultPathForPlatform(test.platform, test.home, test.configDir); got != test.want {
			t.Errorf("defaultPathForPlatform(%q) = %q, want %q", test.platform, got, test.want)
		}
	}
}

func TestDefaultPathHonorsEnvironment(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom.yaml")
	t.Setenv("MONUX_CONFIG", want)
	if got := DefaultPath(); got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}
