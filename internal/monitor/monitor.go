package monitor

import "fmt"

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
}
