package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/dyike/monux/internal/monitor"
)

type fakeBackend struct {
	current  monitor.Input
	displays []monitor.Display
}

func (f *fakeBackend) CurrentInput() (monitor.Input, error) { return f.current, nil }
func (f *fakeBackend) SetInput(input monitor.Input) error {
	f.current = input
	return nil
}
func (f *fakeBackend) Detect() ([]monitor.Display, error) { return f.displays, nil }

func TestStatusCommand(t *testing.T) {
	configPath := prepareCLI(t)
	backend := useFakeBackend(t)
	backend.current = 0x0f

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--config", configPath, "status"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output = %s", err, output.String())
	}
	if got, want := output.String(), "linux (0x0f)\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestSwitchCommand(t *testing.T) {
	configPath := prepareCLI(t)
	backend := useFakeBackend(t)

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--config", configPath, "switch", "mac"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output = %s", err, output.String())
	}
	if got, want := output.String(), "switched to mac (0x11)\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if backend.current != 0x11 {
		t.Fatalf("backend current = %s, want 0x11", backend.current)
	}
}

func TestDetectDoesNotRequireConfig(t *testing.T) {
	backend := useFakeBackend(t)
	backend.displays = []monitor.Display{{ID: "15", Name: "DP-1 (Dell P2415Q)"}}

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--config", filepath.Join(t.TempDir(), "missing.yaml"), "detect"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output = %s", err, output.String())
	}
	if got, want := output.String(), "15\tDP-1 (Dell P2415Q)\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestParseInput(t *testing.T) {
	for value, want := range map[string]uint16{"0x0f": 0x0f, "17": 17, " 0X11 ": 0x11} {
		got, err := parseInput(value)
		if err != nil {
			t.Fatalf("parseInput(%q) error = %v", value, err)
		}
		if uint16(got) != want {
			t.Fatalf("parseInput(%q) = %s, want 0x%02x", value, got, want)
		}
	}
	if _, err := parseInput("not-a-number"); err == nil {
		t.Fatal("parseInput(invalid) error = nil")
	}
}

func prepareCLI(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("monitor:\n  id: 15\ninputs:\n  mac: 0x11\n  linux: 0x0f\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func useFakeBackend(t *testing.T) *fakeBackend {
	t.Helper()
	backend := &fakeBackend{}
	oldFactory := newNativeBackend
	newNativeBackend = func(string) (monitor.Backend, error) { return backend, nil }
	t.Cleanup(func() { newNativeBackend = oldFactory })
	return backend
}
