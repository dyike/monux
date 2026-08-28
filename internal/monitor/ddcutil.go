package monitor

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

const inputSourceVCP = "60"

var (
	shortHexPattern     = regexp.MustCompile(`(?i)\bsl\s*=\s*(0x[0-9a-f]+)\b`)
	currentValuePattern = regexp.MustCompile(`(?i)\bcurrent\s+value\s*=\s*(0x[0-9a-f]+|[0-9]+)\b`)
	terseSNCPattern     = regexp.MustCompile(`(?im)^VCP\s+(?:0x)?60\s+SNC\s+x([0-9a-f]+)\b`)
	terseCPattern       = regexp.MustCompile(`(?im)^VCP\s+(?:0x)?60\s+C\s+(0x[0-9a-f]+|[0-9]+)\b`)
)

type commandRunner func(name string, args ...string) ([]byte, error)

// DDCUtil implements Controller by invoking the ddcutil command-line tool.
type DDCUtil struct {
	bus int
	run commandRunner
}

func NewDDCUtil(bus int) *DDCUtil {
	return &DDCUtil{bus: bus, run: runCommand}
}

// Detect returns ddcutil's brief monitor discovery output.
func (d *DDCUtil) Detect() (string, error) {
	out, err := d.runner()("ddcutil", "detect", "--brief")
	if err != nil {
		return "", commandError("detect monitors", out, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (d *DDCUtil) CurrentInput() (Input, error) {
	if err := d.validateBus(); err != nil {
		return 0, err
	}

	out, err := d.runner()("ddcutil", "--bus", strconv.Itoa(d.bus), "getvcp", inputSourceVCP, "--terse")
	if err != nil {
		return 0, commandError("read monitor input", out, err)
	}

	input, err := parseCurrentInput(string(out))
	if err != nil {
		return 0, fmt.Errorf("parse ddcutil output %q: %w", strings.TrimSpace(string(out)), err)
	}
	return input, nil
}

func (d *DDCUtil) SetInput(input Input) error {
	if err := d.validateBus(); err != nil {
		return err
	}

	out, err := d.runner()("ddcutil", "--bus", strconv.Itoa(d.bus), "setvcp", inputSourceVCP, input.String())
	if err != nil {
		return commandError("set monitor input", out, err)
	}
	return nil
}

func (d *DDCUtil) validateBus() error {
	if d.bus <= 0 {
		return fmt.Errorf("monitor bus must be greater than zero")
	}
	return nil
}

func (d *DDCUtil) runner() commandRunner {
	if d.run != nil {
		return d.run
	}
	return runCommand
}

func runCommand(name string, args ...string) ([]byte, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, fmt.Errorf("%s is not installed or not in PATH: %w", name, err)
	}
	return exec.Command(path, args...).CombinedOutput()
}

func commandError(action string, output []byte, err error) error {
	message := strings.TrimSpace(string(output))
	if message == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, message)
}

func parseCurrentInput(output string) (Input, error) {
	patterns := []struct {
		pattern *regexp.Regexp
		base    int
	}{
		{shortHexPattern, 0},
		{currentValuePattern, 0},
		{terseSNCPattern, 16},
		{terseCPattern, 0},
	}
	for _, candidate := range patterns {
		matches := candidate.pattern.FindStringSubmatch(output)
		if len(matches) != 2 {
			continue
		}
		value, err := strconv.ParseUint(matches[1], candidate.base, 16)
		if err != nil {
			return 0, err
		}
		return Input(value), nil
	}
	return 0, errors.New("input value not found")
}
