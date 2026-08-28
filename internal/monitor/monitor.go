package monitor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dyike/monux/internal/ddc"
)

// Input is the value written to VCP feature 0x60 (Input Source).
type Input uint16

func (i Input) String() string {
	return fmt.Sprintf("0x%02x", uint16(i))
}

func ParseInput(value string) (Input, error) {
	trimmed := strings.TrimSpace(value)
	base := 10
	digits := trimmed
	if strings.HasPrefix(strings.ToLower(trimmed), "0x") {
		base = 16
		digits = trimmed[2:]
	}
	parsed, err := strconv.ParseUint(digits, base, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid input value %q: use decimal or 0x-prefixed hexadecimal", value)
	}
	return Input(parsed), nil
}

func (i Input) ConnectorName() string {
	if name, ok := connectorNames[i]; ok {
		return name
	}
	return "Unknown"
}

var connectorNames = map[Input]string{
	0x01: "VGA 1",
	0x02: "VGA 2",
	0x03: "DVI 1",
	0x04: "DVI 2",
	0x05: "Composite 1",
	0x06: "Composite 2",
	0x07: "S-Video 1",
	0x08: "S-Video 2",
	0x09: "Tuner 1",
	0x0a: "Tuner 2",
	0x0b: "Tuner 3",
	0x0c: "Component 1",
	0x0d: "Component 2",
	0x0e: "Component 3",
	0x0f: "DisplayPort 1",
	0x10: "DisplayPort 2",
	0x11: "HDMI 1",
	0x12: "HDMI 2",
	0x1b: "USB-C",
}

// Controller controls the input source of one monitor.
type Controller interface {
	CurrentInput() (Input, error)
	SetInput(input Input) error
}

type Display struct {
	ID   string
	Name string
}

// Backend is a local, platform-native Controller that can discover displays.
type Backend interface {
	Controller
	Detect() ([]Display, error)
	SupportedInputs() ([]Input, error)
}

func inputsFromCapabilities(capabilities string) ([]Input, error) {
	values, err := ddc.ParseInputCapabilities(capabilities)
	if err != nil {
		return nil, err
	}
	inputs := make([]Input, len(values))
	for i, value := range values {
		inputs[i] = Input(value)
	}
	return inputs, nil
}
