//go:build linux

package monitor

import (
	"os"
	"path/filepath"
	"strings"
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

func TestNativeBackendRediscoversChangedBusByEDID(t *testing.T) {
	root := t.TempDir()
	drmRoot := filepath.Join(root, "drm")
	i2cRoot := filepath.Join(root, "i2c-dev")
	targetEDID := testEDID("DELL P2415Q", 1)
	otherEDID := testEDID("OTHER", 2)
	oldConnector := createLinuxConnector(t, drmRoot, "card0-DP-12", "23", targetEDID, true)
	createLinuxConnector(t, drmRoot, "card0-HDMI-A-1", "7", otherEDID, true)

	backend, err := newNativeBackend("23", drmRoot, i2cRoot)
	if err != nil {
		t.Fatal(err)
	}
	if backend.displayIdentity == "" {
		t.Fatal("newNativeBackend() did not capture the configured display identity")
	}

	if err := os.WriteFile(filepath.Join(oldConnector, "status"), []byte("disconnected\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	createLinuxConnector(t, drmRoot, "card0-DP-1", "0", targetEDID, true)

	if err := backend.rediscoverBus(); err != nil {
		t.Fatalf("rediscoverBus() error = %v", err)
	}
	if backend.bus != 0 {
		t.Fatalf("rediscoverBus() bus = %d, want 0", backend.bus)
	}
}

func TestNativeBackendRediscoversOnlyConnectedDisplayWithoutIdentity(t *testing.T) {
	root := t.TempDir()
	drmRoot := filepath.Join(root, "drm")
	backend, err := newNativeBackend("23", drmRoot, filepath.Join(root, "i2c-dev"))
	if err != nil {
		t.Fatal(err)
	}
	createLinuxConnector(t, drmRoot, "card0-DP-1", "0", testEDID("DELL P2415Q", 1), true)

	if err := backend.rediscoverBus(); err != nil {
		t.Fatalf("rediscoverBus() error = %v", err)
	}
	if backend.bus != 0 {
		t.Fatalf("rediscoverBus() bus = %d, want 0", backend.bus)
	}
}

func TestNativeBackendDoesNotGuessAmongMultipleDisplays(t *testing.T) {
	root := t.TempDir()
	drmRoot := filepath.Join(root, "drm")
	backend, err := newNativeBackend("23", drmRoot, filepath.Join(root, "i2c-dev"))
	if err != nil {
		t.Fatal(err)
	}
	createLinuxConnector(t, drmRoot, "card0-DP-1", "0", nil, true)
	createLinuxConnector(t, drmRoot, "card0-HDMI-A-1", "7", nil, true)

	err = backend.rediscoverBus()
	if err == nil || !strings.Contains(err.Error(), "2 candidate displays matched") {
		t.Fatalf("rediscoverBus() error = %v, want ambiguity error", err)
	}
	if backend.bus != 23 {
		t.Fatalf("rediscoverBus() changed bus to %d after ambiguity", backend.bus)
	}
}

func createLinuxConnector(t *testing.T, drmRoot, name, bus string, edid []byte, connected bool) string {
	t.Helper()
	connector := filepath.Join(drmRoot, name)
	if err := os.MkdirAll(filepath.Join(connector, "i2c-"+bus, "i2c-dev", "i2c-"+bus), 0o755); err != nil {
		t.Fatal(err)
	}
	status := "disconnected\n"
	if connected {
		status = "connected\n"
	}
	if err := os.WriteFile(filepath.Join(connector, "status"), []byte(status), 0o600); err != nil {
		t.Fatal(err)
	}
	if edid != nil {
		if err := os.WriteFile(filepath.Join(connector, "edid"), edid, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return connector
}

func testEDID(name string, marker byte) []byte {
	edid := make([]byte, 128)
	edid[8] = marker
	descriptor := make([]byte, 18)
	copy(descriptor, []byte{0, 0, 0, 0xfc, 0})
	copy(descriptor[5:], name)
	copy(edid[54:72], descriptor)
	return edid
}
