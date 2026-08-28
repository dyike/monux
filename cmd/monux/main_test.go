package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dyike/monux/internal/monitor"
)

type fakeBackend struct {
	current      monitor.Input
	currentErr   error
	displays     []monitor.Display
	supported    []monitor.Input
	supportedErr error
}

func (f *fakeBackend) CurrentInput() (monitor.Input, error) { return f.current, f.currentErr }
func (f *fakeBackend) SetInput(input monitor.Input) error {
	f.current = input
	return nil
}
func (f *fakeBackend) Detect() ([]monitor.Display, error) { return f.displays, nil }
func (f *fakeBackend) SupportedInputs() ([]monitor.Input, error) {
	return f.supported, f.supportedErr
}

func TestInputsCommand(t *testing.T) {
	configPath := prepareCLI(t)
	backend := useFakeBackend(t)
	backend.current = 0x11
	backend.supported = []monitor.Input{0x0f, 0x11, 0x12}

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--config", configPath, "inputs"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output = %s", err, output.String())
	}
	for _, want := range []string{
		"VALUE", "CONNECTOR", "REPORTED", "CURRENT", "NAME",
		"0x0f", "DisplayPort 1", "linux",
		"0x11", "HDMI 1", "mac",
		"0x12", "HDMI 2",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestInputsCommandFallsBackToConfig(t *testing.T) {
	configPath := prepareCLI(t)
	backend := useFakeBackend(t)
	backend.currentErr = errors.New("read failed")
	backend.supportedErr = errors.New("unsupported")

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--config", configPath, "inputs"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output = %s", err, output.String())
	}
	for _, want := range []string{"warning: could not read monitor input capabilities", "0x0f", "linux", "unknown"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestInputsCommandAutoDetectsWithoutConfig(t *testing.T) {
	backend := useFakeBackend(t)
	backend.current = 0x11
	backend.supported = []monitor.Input{0x0f, 0x11}
	backend.displays = []monitor.Display{{ID: "15", Name: "Dell P2415Q"}}

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--config", filepath.Join(t.TempDir(), "missing.yaml"), "inputs"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output = %s", err, output.String())
	}
	for _, want := range []string{"0x0f", "DisplayPort 1", "0x11", "HDMI 1"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestInputsCommandRequiresSelectionForMultipleDetectedMonitors(t *testing.T) {
	backend := useFakeBackend(t)
	backend.displays = []monitor.Display{{ID: "15"}, {ID: "16"}}

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--config", filepath.Join(t.TempDir(), "missing.yaml"), "inputs"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "2 monitors were detected") {
		t.Fatalf("Execute() error = %v, want multiple-monitor selection error", err)
	}
}

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
