package monitor

import (
	"fmt"

	"github.com/dyike/monux/internal/ddc"
)

// Input is the value written to VCP feature 0x60 (Input Source).
type Input uint16

func (i Input) String() string {
	return fmt.Sprintf("0x%02x", uint16(i))
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
