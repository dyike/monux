package service

import (
	"testing"

	"github.com/dyike/monux/internal/monitor"
)

type fakeController struct {
	current monitor.Input
}

func (f *fakeController) CurrentInput() (monitor.Input, error) { return f.current, nil }
func (f *fakeController) SetInput(input monitor.Input) error {
	f.current = input
	return nil
}

func TestSwitch(t *testing.T) {
	controller := &fakeController{}
	switcher := NewSwitcher(controller, map[string]monitor.Input{"mac": 0x11})
	if err := switcher.Switch("mac"); err != nil {
		t.Fatalf("Switch() error = %v", err)
	}
	if controller.current != 0x11 {
		t.Fatalf("current = %s, want 0x11", controller.current)
	}
}

func TestSwitchUnknown(t *testing.T) {
	switcher := NewSwitcher(&fakeController{}, map[string]monitor.Input{"mac": 0x11})
	if err := switcher.Switch("linux"); err == nil {
		t.Fatal("Switch() error = nil, want unknown input error")
	}
}
