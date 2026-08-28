//go:build linux

package monitor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverLinuxDisplays(t *testing.T) {
	root := t.TempDir()
	drmRoot := filepath.Join(root, "drm")
	i2cRoot := filepath.Join(root, "i2c-dev")
	connector := filepath.Join(drmRoot, "card0-DP-1")
	i2cPath := filepath.Join(connector, "ddc", "i2c-dev", "i2c-15")
	if err := os.MkdirAll(i2cPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(connector, "status"), []byte("connected\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	edid := make([]byte, 128)
	copy(edid[54:72], []byte{0, 0, 0, 0xfc, 0, 'D', 'E', 'L', 'L', ' ', 'P', '2', '4', '1', '5', 'Q', '\n', 0})
	if err := os.WriteFile(filepath.Join(connector, "edid"), edid, 0o600); err != nil {
		t.Fatal(err)
	}

	displays, err := discoverLinuxDisplays(drmRoot, i2cRoot)
	if err != nil {
		t.Fatalf("discoverLinuxDisplays() error = %v", err)
	}
	if len(displays) != 1 || displays[0].ID != "15" || displays[0].Name != "card0-DP-1 (DELL P2415Q)" {
		t.Fatalf("discoverLinuxDisplays() = %#v", displays)
	}
}

func TestDiscoverLinuxDisplaysPrefersDisplayPortAUXBus(t *testing.T) {
	root := t.TempDir()
	drmRoot := filepath.Join(root, "drm")
	i2cRoot := filepath.Join(root, "i2c-dev")
	connector := filepath.Join(drmRoot, "card0-DP-1")
	for _, path := range []string{
		filepath.Join(connector, "i2c-23", "i2c-dev", "i2c-23"),
		filepath.Join(connector, "ddc", "i2c-dev", "i2c-17"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(connector, "status"), []byte("connected\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	displays, err := discoverLinuxDisplays(drmRoot, i2cRoot)
	if err != nil {
		t.Fatalf("discoverLinuxDisplays() error = %v", err)
	}
	if len(displays) != 1 || displays[0].ID != "23" || displays[0].Name != "card0-DP-1" {
		t.Fatalf("discoverLinuxDisplays() = %#v", displays)
	}
}

func TestDiscoverLinuxDisplaysFallback(t *testing.T) {
	root := t.TempDir()
	i2cRoot := filepath.Join(root, "i2c-dev")
	for _, id := range []string{"15", "2"} {
		if err := os.MkdirAll(filepath.Join(i2cRoot, "i2c-"+id), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	displays, err := discoverLinuxDisplays(filepath.Join(root, "missing-drm"), i2cRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(displays) != 2 || displays[0].ID != "2" || displays[1].ID != "15" {
		t.Fatalf("discoverLinuxDisplays() = %#v", displays)
	}
}

func TestNewNativeBackendRejectsInvalidID(t *testing.T) {
	if _, err := NewNativeBackend("not-a-bus"); err == nil {
		t.Fatal("NewNativeBackend() error = nil")
	}
}
