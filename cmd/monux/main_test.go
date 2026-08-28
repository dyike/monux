package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusCommand(t *testing.T) {
	configPath, logPath := prepareCLI(t)

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
	assertCommandLog(t, logPath, "--bus 15 getvcp 60 --terse")
}

func TestSwitchCommand(t *testing.T) {
	configPath, logPath := prepareCLI(t)

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--config", configPath, "switch", "mac"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output = %s", err, output.String())
	}
	if got, want := output.String(), "switched to mac (0x11) on bus 15\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	assertCommandLog(t, logPath, "--bus 15 setvcp 60 0x11")
}

func TestDetectDoesNotRequireConfig(t *testing.T) {
	_, logPath := prepareCLI(t)

	cmd := newRootCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--config", filepath.Join(t.TempDir(), "missing.yaml"), "detect"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, output = %s", err, output.String())
	}
	if !strings.Contains(output.String(), "Display 1") {
		t.Fatalf("output = %q, want detected display", output.String())
	}
	assertCommandLog(t, logPath, "detect --brief")
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

func prepareCLI(t *testing.T) (configPath, logPath string) {
	t.Helper()
	dir := t.TempDir()
	configPath = filepath.Join(dir, "config.yaml")
	logPath = filepath.Join(dir, "ddcutil.log")
	configData := []byte("monitor:\n  bus: 15\ninputs:\n  mac: 0x11\n  linux: 0x0f\n")
	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		t.Fatal(err)
	}

	fakePath := filepath.Join(dir, "ddcutil")
	fake := []byte(`#!/bin/sh
printf '%s\n' "$*" > "$DDCUTIL_TEST_LOG"
case "$*" in
  *detect*) printf 'Display 1\n   I2C bus: /dev/i2c-15\n' ;;
  *getvcp*) printf 'VCP 60 SNC x0f\n' ;;
esac
`)
	if err := os.WriteFile(fakePath, fake, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DDCUTIL_TEST_LOG", logPath)
	return configPath, logPath
}

func assertCommandLog(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != want {
		t.Fatalf("ddcutil args = %q, want %q", got, want)
	}
}
