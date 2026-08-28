package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dyike/monux/internal/monitor"
)

type Switcher struct {
	controller monitor.Controller
	inputs     map[string]monitor.Input
}

type NamedInput struct {
	Name  string
	Input monitor.Input
}

func NewSwitcher(controller monitor.Controller, inputs map[string]monitor.Input) *Switcher {
	return &Switcher{controller: controller, inputs: inputs}
}

func (s *Switcher) Switch(name string) error {
	input, ok := s.inputs[name]
	if !ok {
		available := make([]string, 0, len(s.inputs))
		for candidate := range s.inputs {
			available = append(available, candidate)
		}
		sort.Strings(available)
		return fmt.Errorf("unknown input %q (available: %s)", name, strings.Join(available, ", "))
	}
	return s.controller.SetInput(input)
}

func (s *Switcher) Current() (monitor.Input, error) {
	return s.controller.CurrentInput()
}

func (s *Switcher) Name(input monitor.Input) (string, bool) {
	names := make([]string, 0, len(s.inputs))
	for name, candidate := range s.inputs {
		if candidate == input {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "", false
	}
	sort.Strings(names)
	return names[0], true
}

func (s *Switcher) Input(name string) (monitor.Input, bool) {
	input, ok := s.inputs[name]
	return input, ok
}

func (s *Switcher) Inputs() []NamedInput {
	inputs := make([]NamedInput, 0, len(s.inputs))
	for name, input := range s.inputs {
		inputs = append(inputs, NamedInput{Name: name, Input: input})
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Name < inputs[j].Name })
	return inputs
}
