package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dyike/monux/internal/config"
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

func TestInitCommandCreatesDetectedConfig(t *testing.T) {
	backend := useFakeBackend(t)
	backend.displays = []monitor.Display{{ID: "23", Name: "card2-DP-12 (DELL P2415Q)"}}
	backend.current = 0x0f
	backend.supported = []monitor.Input{0x11, 0x10, 0x0f}
	configPath := filepath.Join(t.TempDir(), "monux", "config.yaml")

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--config", configPath, "init"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output = %s", err, output.String())
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Monitor.ID != "23" {
		t.Fatalf("monitor ID = %q, want 23", cfg.Monitor.ID)
	}
	if cfg.Inputs[defaultLocalInputName()] != 0x0f || cfg.Inputs["displayport-2"] != 0x10 || cfg.Inputs["hdmi-1"] != 0x11 {
		t.Fatalf("generated inputs = %#v", cfg.Inputs)
	}
	for _, want := range []string{"created " + configPath, "monitor: 23", "input: " + defaultLocalInputName() + "=0x0f", "(current)", "not the operating system"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output does not contain %q:\n%s", want, output.String())
		}
	}
}

func TestInitCommandRefreshesMonitorAndPreservesNames(t *testing.T) {
	configPath := prepareCLI(t)
	backend := useFakeBackend(t)
	backend.displays = []monitor.Display{{ID: "23", Name: "Dell P2415Q"}}
	backend.current = 0x11
	backend.supported = []monitor.Input{0x0f, 0x10, 0x11}

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--config", configPath, "init"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output = %s", err, output.String())
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Monitor.ID != "23" || cfg.Inputs["linux"] != 0x0f || cfg.Inputs["mac"] != 0x11 {
		t.Fatalf("refreshed config = %#v", cfg)
	}
	if cfg.Inputs["displayport-2"] != 0x10 {
		t.Fatalf("missing discovered input: %#v", cfg.Inputs)
	}
	if !strings.Contains(output.String(), "updated "+configPath) {
		t.Fatalf("output = %s", output.String())
	}
}

func TestInitCommandAcceptsNamedInputOverrides(t *testing.T) {
	backend := useFakeBackend(t)
	backend.displays = []monitor.Display{{ID: "1", Name: "Dell P2415Q"}}
	backend.current = 0x11
	backend.supported = []monitor.Input{0x0f, 0x11}
	configPath := filepath.Join(t.TempDir(), "config.yaml")

	cmd := newRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", configPath, "init", "--input", "mac=0x11", "--input", "linux=0x0f"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Inputs) != 2 || cfg.Inputs["mac"] != 0x11 || cfg.Inputs["linux"] != 0x0f {
		t.Fatalf("generated inputs = %#v", cfg.Inputs)
	}
}

func TestInitCommandRequiresMonitorSelection(t *testing.T) {
	backend := useFakeBackend(t)
	backend.displays = []monitor.Display{{ID: "15", Name: "First"}, {ID: "23", Name: "Second"}}
	backend.current = 0x0f
	backend.supported = []monitor.Input{0x0f}
	configPath := filepath.Join(t.TempDir(), "config.yaml")

	cmd := newRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", configPath, "init"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--monitor <id>") {
		t.Fatalf("Execute() error = %v, want monitor selection error", err)
	}

	cmd = newRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", configPath, "init", "--monitor", "23"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() selected monitor error = %v", err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Monitor.ID != "23" {
		t.Fatalf("monitor ID = %q, want 23", cfg.Monitor.ID)
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
